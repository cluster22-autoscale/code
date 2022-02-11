package main

import (
	"encoding/json"
	"fmt"
	"github.com/iwqos22-autoscale/code/mock/utils"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"
	"time"
)

var httpRequestCount = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "http_request_total",
		Help: "http request total",
	},
	[]string{"endpoint"},
)

func dealWithOption(option *utils.Option) {
	for i := 0; i < option.Length; i++ {
		i := i
		go func() {
			operation := option.Operations[i]

			until := time.NewTimer(option.Duration)
			t := utils.DefaultInterval * float64(1000) / float64(option.Rates[i])
			duration, _ := time.ParseDuration(fmt.Sprintf("%f", t) + "ms")
			cron := time.NewTicker(duration)
			for {
				select {
				case <-until.C:
					return
				case <-cron.C:
					httpRequestCount.WithLabelValues(operation).Add(utils.DefaultInterval)
				}
			}
		}()
	}
}

func handler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		w.WriteHeader(500)
	}

	var config utils.Config
	if err := json.Unmarshal([]byte(r.PostForm.Get("config")), &config);
		err != nil {
		return
	}
	option := utils.NewForConfig(&config)
	dealWithOption(option)

	w.WriteHeader(200)
}

func init() {
	prometheus.MustRegister(httpRequestCount)
}

func main() {
	http.Handle("/metrics", promhttp.Handler())
	http.Handle("/indicator", http.HandlerFunc(handler))
	if err := http.ListenAndServe(":30576", nil); err != nil {
		fmt.Println(err)
	}
}
