package metrics

import (
	"context"
	"fmt"
	"github.com/iwqos22-autoscale/code/utils"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
)

const (
	defaultPrometheusAddress    string = "http://localhost:30090"
	defaultTimeStepForRangQuery        = 1 * time.Second
	defaultTimeOutForQuery             = 10 * time.Second
	defaultIntervalMetrics             = 5 * time.Second
	defaultNameSpace            string = "social-network"
)

type MetricsMonitor struct {
	promClient *api.Client
}

func NewMetricsMonitor() *MetricsMonitor {
	config := api.Config{Address: defaultPrometheusAddress}
	client, err := api.NewClient(config)
	if err != nil {
		fmt.Printf("Error creating client: %v\n", err)
		os.Exit(1)
	}

	return &MetricsMonitor{
		promClient: &client,
	}
}

func (m *MetricsMonitor) ExtractResourceType(podName string, t time.Time) utils.ResourceType {
	queries := map[utils.ResourceType]string{
		utils.ResourceCPU: fmt.Sprintf(`sum(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_rate
			{namespace="%s", pod=~"%s", container!=""}) by (pod)`,
			defaultNameSpace, podName),
		utils.ResourceMemory: fmt.Sprintf(`sum(container_memory_working_set_bytes
			{namespace="%s", pod=~"%s", container!="", image!=""}) by (pod)`,
			defaultNameSpace, podName),
		utils.ResourceNetworkBandwidth: fmt.Sprintf(`sum(irate(container_network_receive_bytes_total
			{namespace="%s", pod=~"%s", container!="", image!=""}[30s:15s])) by (pod)`,
			defaultNameSpace, podName),
	}

	bottleneck := utils.ResourceCPU
	maxGradient := 0.0
	for type_, query := range queries {
		// if defaultIntervalMetrics == 5s, len(results) == 6
		results := m.MetricsForTimeRange(query, t.Add(-defaultIntervalMetrics), t)
		mean := calculateMean(results)
		if mean == 0.0 {
			continue
		}
		gradient := calculateGradient(results)
		relativeGradient := gradient / mean
		if relativeGradient > maxGradient {
			bottleneck = type_
			maxGradient = relativeGradient
		}
	}
	return bottleneck
}

// MetricsForTimeRange 查询cpu、memory、nb、rps等指标
func (m *MetricsMonitor) MetricsForTimeRange(query string, timeStart time.Time, timeEnd time.Time) []float64 {
	v1api := promv1.NewAPI(*m.promClient)
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeOutForQuery)
	defer cancel()

	r := promv1.Range{
		Start: timeStart,
		End:   timeEnd,
		Step:  defaultTimeStepForRangQuery,
	}
	result, warnings, err := v1api.QueryRange(ctx, query, r)
	if err != nil {
		fmt.Printf("Error querying Prometheus: %v\n", err)
		os.Exit(1)
	}
	if len(warnings) > 0 {
		fmt.Printf("Warnings: %v\n", warnings)
	}

	return parseResults(result.String())
}

func (m *MetricsMonitor) MetricsForTime(query string, t time.Time) float64 {
	v1api := promv1.NewAPI(*m.promClient)
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeOutForQuery)
	defer cancel()

	result, warnings, err := v1api.Query(ctx, query, t)
	if err != nil {
		fmt.Printf("Error querying Prometheus: %v\n", err)
		os.Exit(1)
	}
	if len(warnings) > 0 {
		fmt.Printf("Warnings: %v\n", warnings)
	}

	return parseResult(result.String())
}

func parseResults(s string) []float64 {
	results := make([]float64, 0)
	re := regexp.MustCompile("\n.* @")
	bufs := re.FindAll([]byte(s), -1)
	for _, buf := range bufs {
		result, _ := strconv.ParseFloat(string(buf[1:len(buf)-2]), 64)
		results = append(results, result)
	}
	return results
}

func parseResult(s string) float64 {
	re := regexp.MustCompile("=> .* @")
	buf := re.Find([]byte(s))
	result, _ := strconv.ParseFloat(string(buf[3:len(buf)-2]), 64)
	return result
}

func calculateMean(nums []float64) float64 {
	sum := 0.0
	for _, num := range nums {
		sum += num
	}
	return sum / float64(len(nums))
}

func calculateGradient(nums []float64) float64 {
	// if defaultIntervalMetrics == 5s, len(results) == 6
	step := defaultIntervalMetrics.Seconds()
	g1 := (nums[3] - nums[0]) / step
	g2 := (nums[4] - nums[1]) / step
	g3 := (nums[5] - nums[2]) / step
	return (g1 + g2 + g3) / 3
}
