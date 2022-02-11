package utils

type ResourceType string

const (
	ResourceCPU              ResourceType = "cpu"
	ResourceMemory           ResourceType = "memory"
	ResourceNetworkBandwidth ResourceType = "network-bandwidth"
	ResourceReplica          ResourceType = "replica"
)

type ResourceAmount uint64

type ResourceList map[ResourceType]ResourceAmount

type ContainerName string

type ResourceMap map[ContainerName]ResourceList
