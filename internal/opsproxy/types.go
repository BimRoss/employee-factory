package opsproxy

type Operation string

const (
	OperationK8sStatus      Operation = "k8s_status"
	OperationK8sMetrics     Operation = "k8s_metrics"
	OperationK8sLogs        Operation = "k8s_logs"
	OperationRedisRead      Operation = "redis_read"
	OperationWaitlistEmails Operation = "waitlist_emails"
)

type StatusRequest struct {
	Namespace string `json:"namespace,omitempty"`
	Target    string `json:"target,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

type DeploymentStatus struct {
	Namespace         string   `json:"namespace"`
	Name              string   `json:"name"`
	Replicas          int32    `json:"replicas"`
	ReadyReplicas     int32    `json:"ready_replicas"`
	UpdatedReplicas   int32    `json:"updated_replicas"`
	AvailableReplicas int32    `json:"available_replicas"`
	Images            []string `json:"images,omitempty"`
}

type PodStatus struct {
	Namespace string   `json:"namespace"`
	Name      string   `json:"name"`
	Phase     string   `json:"phase"`
	Ready     bool     `json:"ready"`
	Restarts  int32    `json:"restarts"`
	NodeName  string   `json:"node_name,omitempty"`
	Images    []string `json:"images,omitempty"`
}

type StatusResponse struct {
	Namespace   string             `json:"namespace,omitempty"`
	Target      string             `json:"target,omitempty"`
	Deployments []DeploymentStatus `json:"deployments,omitempty"`
	Pods        []PodStatus        `json:"pods,omitempty"`
}

type MetricsRequest struct {
	Namespace string `json:"namespace,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

type ClusterResourceTotals struct {
	CPUCapacityMilli       int64 `json:"cpu_capacity_milli"`
	CPUAllocatableMilli    int64 `json:"cpu_allocatable_milli"`
	CPURequestedMilli      int64 `json:"cpu_requested_milli"`
	CPULimitsMilli         int64 `json:"cpu_limits_milli"`
	CPUUsageMilli          int64 `json:"cpu_usage_milli"`
	MemoryCapacityBytes    int64 `json:"memory_capacity_bytes"`
	MemoryAllocatableBytes int64 `json:"memory_allocatable_bytes"`
	MemoryRequestedBytes   int64 `json:"memory_requested_bytes"`
	MemoryLimitsBytes      int64 `json:"memory_limits_bytes"`
	MemoryUsageBytes       int64 `json:"memory_usage_bytes"`
}

type NodeResourceMetrics struct {
	NodeName               string `json:"node_name"`
	CPUCapacityMilli       int64  `json:"cpu_capacity_milli"`
	CPUAllocatableMilli    int64  `json:"cpu_allocatable_milli"`
	CPURequestedMilli      int64  `json:"cpu_requested_milli"`
	CPULimitsMilli         int64  `json:"cpu_limits_milli"`
	CPUUsageMilli          int64  `json:"cpu_usage_milli"`
	MemoryCapacityBytes    int64  `json:"memory_capacity_bytes"`
	MemoryAllocatableBytes int64  `json:"memory_allocatable_bytes"`
	MemoryRequestedBytes   int64  `json:"memory_requested_bytes"`
	MemoryLimitsBytes      int64  `json:"memory_limits_bytes"`
	MemoryUsageBytes       int64  `json:"memory_usage_bytes"`
}

type MetricsResponse struct {
	Namespace            string                `json:"namespace,omitempty"`
	LiveMetricsAvailable bool                  `json:"live_metrics_available"`
	LiveMetricsReason    string                `json:"live_metrics_reason,omitempty"`
	Cluster              ClusterResourceTotals `json:"cluster"`
	Nodes                []NodeResourceMetrics `json:"nodes,omitempty"`
}

type LogsRequest struct {
	Namespace    string `json:"namespace,omitempty"`
	Target       string `json:"target,omitempty"`
	Container    string `json:"container,omitempty"`
	TailLines    int64  `json:"tail_lines,omitempty"`
	SinceSeconds int64  `json:"since_seconds,omitempty"`
}

type LogsResponse struct {
	Namespace string `json:"namespace"`
	Target    string `json:"target"`
	Lines     string `json:"lines"`
	Truncated bool   `json:"truncated"`
}

type RedisReadRequest struct {
	Key    string `json:"key,omitempty"`
	Prefix string `json:"prefix,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

type RedisItem struct {
	Key   string `json:"key"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

type RedisReadResponse struct {
	Items []RedisItem `json:"items"`
}

type WaitlistEmailsRequest struct {
	Prefix     string `json:"prefix,omitempty"`
	Limit      int    `json:"limit,omitempty"`
	RevealFull bool   `json:"reveal_full,omitempty"`
}

type WaitlistEmail struct {
	Email     string `json:"email"`
	UpdatedAt string `json:"updated_at,omitempty"`
	SourceKey string `json:"source_key,omitempty"`
	// Source is kept for backward compatibility with existing clients.
	Source string `json:"source,omitempty"`
}

type WaitlistEmailsResponse struct {
	Emails           []WaitlistEmail `json:"emails"`
	SearchedPrefixes []string        `json:"searched_prefixes,omitempty"`
}
