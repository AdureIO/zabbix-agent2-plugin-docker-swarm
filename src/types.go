package main

// Service represents a Docker Swarm service.
type Service struct {
	ID   string      `json:"ID"`
	Spec ServiceSpec `json:"Spec"`
}

// ServiceSpec represents the specification of a service.
type ServiceSpec struct {
	Name         string            `json:"Name"`
	Mode         ServiceMode       `json:"Mode"`
	Labels       map[string]string `json:"Labels"`
	TaskTemplate TaskTemplate      `json:"TaskTemplate"`
}

// TaskTemplate holds the task configuration, including placement rules.
type TaskTemplate struct {
	Placement *Placement `json:"Placement"`
}

// Placement holds placement constraints and per-node replica caps.
type Placement struct {
	Constraints []string `json:"Constraints"`
	MaxReplicas uint64   `json:"MaxReplicas"`
}

// ServiceMode represents the mode of a service (replicated or global).
type ServiceMode struct {
	Replicated *ReplicatedService `json:"Replicated"`
	Global     *GlobalService     `json:"Global"`
}

// ReplicatedService is for a replicated service.
type ReplicatedService struct {
	Replicas *uint64 `json:"Replicas"`
}

// GlobalService is for a global service.
type GlobalService struct{}

// Task represents a task running as part of a service.
type Task struct {
	ID           string     `json:"ID"`
	ServiceID    string     `json:"ServiceID"`
	NodeID       string     `json:"NodeID"`
	Slot         int        `json:"Slot"`
	Status       TaskStatus `json:"Status"`
	DesiredState string     `json:"DesiredState"`
}

// Node represents a node in the swarm cluster.
type Node struct {
	ID          string          `json:"ID"`
	Spec        NodeSpec        `json:"Spec"`
	Description NodeDescription `json:"Description"`
	Status      NodeStatus      `json:"Status"`
}

// NodeSpec holds the operator-configured properties of a node.
type NodeSpec struct {
	Role         string            `json:"Role"`
	Availability string            `json:"Availability"`
	Labels       map[string]string `json:"Labels"`
}

// NodeDescription holds hardware and engine details reported by the node.
type NodeDescription struct {
	Hostname string            `json:"Hostname"`
	Platform NodePlatform      `json:"Platform"`
	Engine   EngineDescription `json:"Engine"`
}

// NodePlatform holds OS and architecture information.
type NodePlatform struct {
	OS           string `json:"OS"`
	Architecture string `json:"Architecture"`
}

// EngineDescription holds engine-level labels.
type EngineDescription struct {
	Labels []string `json:"Labels"`
}

// NodeStatus holds the current connectivity state of the node.
type NodeStatus struct {
	State string `json:"State"`
	Addr  string `json:"Addr"`
}

// TaskStatus represents the status of a task.
type TaskStatus struct {
	State           string               `json:"State"`
	Timestamp       string               `json:"Timestamp"`
	ContainerStatus *TaskContainerStatus `json:"ContainerStatus,omitempty"`
}

// TaskContainerStatus contains container-specific status information.
type TaskContainerStatus struct {
	ContainerID string `json:"ContainerID"`
	ExitCode    int    `json:"ExitCode"`
}

// DockerInfo contains the relevant subset of /info response.
type DockerInfo struct {
	Swarm SwarmInfo `json:"Swarm"`
}

// SwarmInfo holds the node's swarm identity.
type SwarmInfo struct {
	NodeID string `json:"NodeID"`
}

// ContainerStats is the response from /containers/{id}/stats?stream=false.
type ContainerStats struct {
	CPUStats    CPUStats    `json:"cpu_stats"`
	PreCPUStats CPUStats    `json:"precpu_stats"`
	MemoryStats MemoryStats `json:"memory_stats"`
}

// CPUStats holds CPU accounting for one sample point.
type CPUStats struct {
	CPUUsage       CPUUsage `json:"cpu_usage"`
	SystemCPUUsage uint64   `json:"system_cpu_usage"`
	OnlineCPUs     int      `json:"online_cpus"`
}

// CPUUsage holds per-CPU and total CPU usage in nanoseconds.
type CPUUsage struct {
	TotalUsage  uint64   `json:"total_usage"`
	PercpuUsage []uint64 `json:"percpu_usage"`
}

// MemoryStats holds memory usage and limits for a container.
type MemoryStats struct {
	Usage uint64            `json:"usage"`
	Limit uint64            `json:"limit"`
	Stats map[string]uint64 `json:"stats"`
}

// ReplicaStats is the JSON payload returned by swarm.replica.stats.
type ReplicaStats struct {
	CPUPercent float64 `json:"cpu_percent"`
	CPUNs      uint64  `json:"cpu_ns"`
	MemBytes   uint64  `json:"mem_bytes"`
	MemPercent float64 `json:"mem_percent"`
	MemLimit   uint64  `json:"mem_limit"`
}

// NodeStatusResult is the JSON payload returned by swarm.node.status.
type NodeStatusResult struct {
	Hostname     string `json:"hostname"`
	Role         string `json:"role"`
	Availability string `json:"availability"`
	State        string `json:"state"`
	Addr         string `json:"addr"`
}

// ErrorMessage represents the API error message from Docker.
type ErrorMessage struct {
	Message string `json:"message"`
}
