## Codes of IWQoS'22 Autoscaler

### Prerequisites

1. [Kubernetes cluster](https://kubernetes.io/zh/docs/setup/production-environment/tools/kubeadm/create-cluster-kubeadm/) available.
2. [Prometheus operator](https://github.com/prometheus-operator/prometheus-operator) deployed.
3. Jaeger tracing deployed with BadgerDB as [storage backend](https://www.jaegertracing.io/docs/1.31/deployment/#badger---local-storage).

### Codes Description

1. `extractor`: Communicating with Jaeger tracing and building DAG.
2. `metrics`: Communicating with Prometheus for runtime metrics.
3. `mock`: Simulating RPS query, working together with `metrics`.
4. `updator`: Updating resource allocation.
5. `benchmarks`: Benchmarks.

### Executable program entries

1. `main.go`: The main program.
2. `updator/server/server.go`: The grpc servers to update cgroups files.
3. `mock/cmd/client.go`: The client to simulate http requests only for metrics monitoring.
4. `mock/exporter/server.go`: The HTTP RPS exporter of Prometheus.

The building process can be found in [Makefile](./Makefile).
For details, please check README in subdirectories.

### Benchmark

The benchmark is forked from SocialNetwork in [DeathStarBench](https://github.com/delimitrou/DeathStarBench).

#### Deployment

1. Deploy social network.
```shell
cd ./benchmarks/socialNetwork/k8s-yaml
kubectl apply -f ./setup/.
kubectl apply -f ./backend/.
kubectl apply -f .
```

Waiting for all pods ready.

2. Deploy the auto scaler.

Change `ipMap` and `defaultStorePath` in `updator/updator.go` to your values.
Then run `make` to build all executable files and images.

```shell
make
```

The executable files are in `bin/`.

Deploy `bin/updator` on each worker node of K8S cluster.

Deploy `bin/mock_exporter` in a Pod.

```shell
cd mock/yamls
kubectl apply -f ./exporter.yaml
kubectl apply -f ./podMonitor.yaml
```

Waiting for that the `mock_exporter` is monitored by Prometheus.

Deploy `bin/main` on the same node with BadgerDB storage path. 

#### Experiments

1. Generate workloads.

```shell
ip=`kubectl -n social-network get svc nginx-thrift | awk '{print $3}' | sed -n '2p'`
cd benchmarks/socialNetwork/
python3 ./scripts/init_social_graph.py --ip $ip
```

2. Running experiments.

```shell
cd benchmarks/socialNetwork/wrk2
ip=`kubectl get pod -n social-network -owide | grep nginx | awk {'print $6'}`
./wrk -D exp -t <num-threads> -c <num-connections> -d <duration> -R <rps> -L -s ./scripts/social-network/mixed-workload.lua http://$ip:8080/
```
