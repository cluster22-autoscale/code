## The mock exporter

This is an exporter for Prometheus.
It serves the request from mock client and simulate the rps of http requests.
The Prometheus server will pull metrics from this exporter.

### Deployment

Note that for Kubernetes, this exporter should be deployed as a Pod.
The deployment files are in [yamls](../yamls).
