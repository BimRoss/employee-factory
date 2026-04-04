package opsproxy

type Operation string

const (
	OperationK8sStatus Operation = "k8s_status"
	OperationK8sLogs   Operation = "k8s_logs"
	OperationRedisRead Operation = "redis_read"
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
