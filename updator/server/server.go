package main

import (
	"context"
	"fmt"
	"github.com/iwqos22-autoscale/code/updator"
	"github.com/iwqos22-autoscale/code/utils"
	"io/ioutil"
	"net"
	"strconv"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type server struct{}

func (s *server) DoUpdate(_ context.Context, in *updator.UpdateRequest) (*updator.UpdateReply, error) {
	podName := in.GetPodName()
	delta := float64(in.GetDelta())
	resourceType := in.GetResourceType()
	switch utils.ResourceType(resourceType) {
	case utils.ResourceCPU:
		return &updator.UpdateReply{LatestShare: updateCpu(podName, delta)}, nil
	case utils.ResourceMemory:
		return &updator.UpdateReply{LatestShare: updateMemory(podName, delta)}, nil
	case utils.ResourceNetworkBandwidth:
		return &updator.UpdateReply{LatestShare: updateNetworkBandwidth(podName, delta)}, nil
	default:
		return &updator.UpdateReply{LatestShare: 0},
			fmt.Errorf("No such type: %s\n", resourceType)
	}
}

func updateCpu(containerID string, delta float64) int64 {
	path := "/sys/fs/cgroup/cpu/kubepods/pod" + containerID + "/cpu.cfs_quota_us"
	content, err := ioutil.ReadFile(path)
	if err != nil {
		panic(err)
	}
	curr, _ := strconv.ParseFloat(string(content), 64)
	newValue := int64(curr * (1 + delta))
	newContent := strconv.FormatInt(newValue, 10)
	err = ioutil.WriteFile(path, []byte(newContent), 0644)
	if err != nil {
		panic(err)
	}
	return newValue
}

func updateMemory(containerID string, delta float64) int64 {
	path := "/sys/fs/cgroup/memory/kubepods/pod" + containerID + "/memory.limit_in_bytes"
	content, err := ioutil.ReadFile(path)
	if err != nil {
		panic(err)
	}
	curr, _ := strconv.ParseFloat(string(content), 64)
	newValue := int64(curr * (1 + delta))
	newContent := strconv.FormatInt(newValue, 10)
	err = ioutil.WriteFile(path, []byte(newContent), 0644)
	if err != nil {
		panic(err)
	}
	return newValue
}

func updateNetworkBandwidth(containerID string, delta float64) int64 {
	// TODO: network
	path := "/sys/fs/cgroup/cpu/kubepods/pod" + containerID + "/?"
	content, err := ioutil.ReadFile(path)
	if err != nil {
		panic(err)
	}
	curr, _ := strconv.ParseFloat(string(content), 64)
	newValue := int64(curr * (1 + delta))
	newContent := strconv.FormatInt(newValue, 10)
	err = ioutil.WriteFile(path, []byte(newContent), 0644)
	if err != nil {
		panic(err)
	}
	return newValue
}

func main() {
	listen, err := net.Listen("tcp", ":8972")
	if err != nil {
		fmt.Printf("failed to listen: %v", err)
		return
	}
	s := grpc.NewServer()
	updator.RegisterUpdateServer(s, &server{})
	reflection.Register(s)

	err = s.Serve(listen)
	if err != nil {
		fmt.Printf("failed to serve: %v", err)
		return
	}
}
