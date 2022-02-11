package main

import (
	"github.com/iwqos22-autoscale/code/updator"
	"time"
)

const (
	defaultInterval = 10 * time.Second
)

func main() {
	updater := updator.NewUpdator()
	ticker := time.Tick(defaultInterval)
	for range ticker {
		updater.RunOnce()
	}
}
