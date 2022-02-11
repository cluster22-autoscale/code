package updator

import (
	"context"
	"flag"
	"fmt"
	"github.com/jaegertracing/jaeger/model"
	"path/filepath"
	"sort"
	"time"

	"google.golang.org/grpc"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	"github.com/iwqos22-autoscale/code/extractor"
	"github.com/iwqos22-autoscale/code/metrics"
	"github.com/iwqos22-autoscale/code/utils"
)

const (
	defaultNamespace        string  = "social-network"
	defaultStorePath        string  = "/path/to/badgerdb"
	defaultIntervalBefore           = 10 * time.Second
	defaultIntervalAfter            = 10 * time.Second
	defaultIntervalChecking         = 5 * time.Second
	defaultNumTraces        int     = 1000
	defaultQoSThreshold     float64 = 1000.0
	defaultE2eLatency               = 1 * time.Second
	defaultRPSThreshold     int64   = 1000
	defaultIntervalScan             = 5 * time.Second
)

type policyKey struct {
	t          utils.ResourceType
	isHighLoad bool
}

var (
	ipMap = map[string]string{
		"node1": "192.168.1.107",
		"node2": "192.168.1.104",
		"node3": "192.168.1.118",
		"node4": "192.168.1.119",
		"node5": "192.168.1.114",
		"node6": "192.168.1.109",
		"node7": "192.168.1.117",
	}
	policyMap = map[policyKey]utils.ResourceType{
		{utils.ResourceCPU, false}:              utils.ResourceCPU,
		{utils.ResourceCPU, true}:               utils.ResourceReplica,
		{utils.ResourceMemory, false}:           utils.ResourceMemory,
		{utils.ResourceMemory, true}:            utils.ResourceMemory,
		{utils.ResourceNetworkBandwidth, false}: utils.ResourceNetworkBandwidth,
		{utils.ResourceNetworkBandwidth, true}:  utils.ResourceReplica,
	}
)

type HistoryEntry struct {
	currRps   int64
	currShare int64
	quality float64
}

// Updator string为podName
type Updator struct {
	history        map[string]*HistoryEntry
	clientset      *kubernetes.Clientset
	metricsMonitor *metrics.MetricsMonitor
	traceReader    *extractor.TraceReader
	svcList        []string
	svcPodsMap     map[string]*[]string
}

func NewUpdator() *Updator {
	var kubeconfig *string

	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "")
	}
	flag.Parse()

	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	clientset, err := kubernetes.NewForConfig(config)

	if err != nil {
		panic(err)
	}

	monitor := metrics.NewMetricsMonitor()
	traceReader := extractor.NewTraceReader(defaultStorePath)

	return &Updator{
		history:        make(map[string]*HistoryEntry),
		clientset:      clientset,
		metricsMonitor: monitor,
		traceReader:    traceReader,
		svcList:        []string{},
		svcPodsMap:     make(map[string]*[]string, 0),
	}
}

// make sure: timeStart < timeEnd
func (u *Updator) getQoS(svcName string, timeStart, timeEnd time.Time) (time.Duration, time.Duration) {
	// 注意这里用的是jaeger，用svcName来查，也即span.Process.ServiceName，而非k8s svc。
	query := extractor.NewQuery(svcName, timeStart, timeEnd, defaultNumTraces)
	tracesIDs, err := u.traceReader.QueryTimeRange(query)
	if err != nil {
		panic(err)
	}

	traces, err := u.traceReader.GetTraces(tracesIDs)
	if err != nil {
		panic(err)
	}

	lat50And99 := u.traceReader.GetPercentileLatency([]float64{0.5, 0.99}, traces,
		func(span *model.Span) bool {
			return span.Process.ServiceName == svcName
		})
	return lat50And99[0], lat50And99[1]
}

func (u *Updator) getQoSByOperation(svcName string, opNames []string, timeStart, timeEnd time.Time) map[string][]time.Duration {
	query := extractor.NewQuery(svcName, timeStart, timeEnd, defaultNumTraces)
	tracesIDs, err := u.traceReader.QueryTimeRange(query)
	if err != nil {
		panic(err)
	}

	traces, err := u.traceReader.GetTraces(tracesIDs)
	if err != nil {
		panic(err)
	}

	lat50And99s := u.traceReader.GetPercentileLatencyByOperation([]float64{0.5, 0.99}, traces, opNames)
	return lat50And99s
}

func nodeNametoIP(nodeName string) string {
	return ipMap[nodeName]
}

func getPolicy(bottleneck utils.ResourceType, rps int64) utils.ResourceType {
	return policyMap[policyKey{bottleneck, rps > defaultRPSThreshold}]
}

func (u *Updator) update(podName string, rps int64) {
	timeNow := time.Now()
	lat50Before, lat99Before := u.getQoS(podName2SvcName(podName), timeNow.Add(-defaultIntervalBefore), timeNow)

	var delta float64
	if _, ok := u.history[podName]; ok {
		history := u.history[podName]
		lastRps := history.currRps
		ratio := float64(rps) / float64(lastRps)
		quality := history.quality
		delta = float64(CalculateDelta(ratio, quality))
		u.history[podName].currRps = rps
	} else {
		delta = 1.0
		u.history[podName] = &HistoryEntry{
			currRps: rps,
		}
	}

	bottleneck := u.metricsMonitor.ExtractResourceType(podName, timeNow)
	policy := getPolicy(bottleneck, rps)

	var latestShare int64
	if policy == utils.ResourceReplica {
		latestShare = u.updateReplica(podName2SvcName(podName), delta)
	} else {
		pod, err := u.clientset.CoreV1().Pods(defaultNamespace).Get(context.Background(), podName, metav1.GetOptions{})
		if err != nil {
			fmt.Printf("failed to get pod: %v", podName)
		}

		remoteAddress := nodeNametoIP(pod.Spec.NodeName) + ":8972"
		conn, err := grpc.Dial(remoteAddress, grpc.WithInsecure())
		if err != nil {
			fmt.Printf("faild to connect: %v", err)
		}
		defer conn.Close()

		podUID := pod.ObjectMeta.UID
		containerUID := pod.Status.ContainerStatuses[0].ContainerID[9:]
		targetPath := fmt.Sprintf("%s/%s", podUID, containerUID)

		c := NewUpdateClient(conn)
		reply, err := c.DoUpdate(context.Background(), &UpdateRequest{
			PodName:      targetPath,
			Delta:        float32(delta),
			ResourceType: string(policy),
		})

		latestShare = reply.LatestShare
	}
	u.history[podName].currShare = latestShare

	timeNow = time.Now()
	lat50After, lat99After := u.getQoS(podName2SvcName(podName), timeNow, timeNow.Add(defaultIntervalAfter))
	u.history[podName].quality = (float64(lat99After) / float64(lat50After)) / (float64(lat99Before) / float64(lat50Before))
}

func (u *Updator) updateReplica(serviceName string, delta float64) int64 {
	ss := u.clientset.AppsV1().StatefulSets(defaultNamespace)
	oldScale, err := ss.GetScale(context.Background(), serviceName, metav1.GetOptions{})
	if err != nil {
		panic(err)
	}
	oldReplicas := oldScale.Spec.Replicas
	newReplicas := int32(float64(oldReplicas) * delta)
	newScale := oldScale.DeepCopy()
	newScale.Spec.Replicas = newReplicas
	ss.UpdateScale(context.Background(), serviceName, newScale, metav1.UpdateOptions{})
	return int64(newReplicas)
}

func (u *Updator) svcName2PodName(svcName string) string {
	return (*u.svcPodsMap[svcName])[0]
}

func podName2SvcName(podName string) string {
	// podName: svcName-aaaaaaaaaa-aaaaa
	return podName[:len(podName)-17]
}

func (u *Updator) isQosViolation() (bool, string) {
	timeNow := time.Now()
	opNames := []string{"/wrk2-api/post/compose", "/wrk2-api/user-timeline/read", "/wrk2-api/home-timeline/read"}
	lat50and99s := u.getQoSByOperation("nginx-web-server", opNames, timeNow.Add(-defaultIntervalChecking), timeNow)

	var violation bool
	var operation string
	var prevLat time.Duration
	for op, lats := range lat50and99s {
		lat50, lat99 := lats[0], lats[1]
		violation = lat50 > defaultE2eLatency || float64(lat99)/float64(lat50) > defaultQoSThreshold
		if violation && lat99 > prevLat {
			prevLat = lat99
			operation = op
		}
	}
	return violation, operation
}

func (u *Updator) ExtractBottleNeckPod() string {
	t := time.Now()
	query := extractor.NewQuery("", t.Add(-defaultIntervalScan), t, defaultNumTraces)
	traceIDs, err := u.traceReader.QueryTimeRange(query)
	if err != nil {
		fmt.Println("can not get traceIDs")
	}

	traces, err := u.traceReader.GetTraces(traceIDs)
	if err != nil {
		fmt.Println("can not get traces")
	}

	pathSet := make(map[*extractor.Path]struct{}, 0)
	for _, trace := range traces {
		graph := extractor.NewGraph(trace)
		path := graph.GetLongestPath()
		if _, exists := pathSet[path]; exists {
			continue
		} else {
			pathSet[path] = struct{}{}
		}
	}

	bottlenecks := make(map[string][]time.Duration)
	for p := range pathSet {
		curr := p.GetHead()
		for curr != nil {
			span := curr.GetSpan()
			pod := span.GetPodName()
			duration := span.GetDuration()
			bottlenecks[pod] = append(bottlenecks[pod], duration)
			curr = curr.GetNext()
		}
	}

	var bottleneck string
	max := 0.0
	for pod, latencies := range bottlenecks {
		sort.Slice(latencies, func(i, j int) bool {
			return latencies[i] < latencies[j]
		})
		length := float64(len(latencies))
		lat50 := latencies[int(length*0.5)]
		lat99 := latencies[int(length*0.99)]
		qos := float64(lat99) / float64(lat50)
		if qos > max {
			max = qos
			bottleneck = pod
		}
	}

	return bottleneck
}

func (u *Updator) RunOnce() {
	if violation, opName := u.isQosViolation(); violation {
		rpsQuery := fmt.Sprintf(`rate(http_request_total{exported_endpoint="%s"}[2s])`, opName)
		rps := u.metricsMonitor.MetricsForTime(rpsQuery, time.Now())
		podName := u.ExtractBottleNeckPod()
		go u.update(podName, int64(rps))
	}
}

