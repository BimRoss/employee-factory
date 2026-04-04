package opsproxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/redis/go-redis/v9"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type ProxyServer struct {
	cfg            *ProxyConfig
	kube           kubernetes.Interface
	redis          *redis.Client
	namespaceAllow map[string]struct{}
}

func NewProxyServer(cfg *ProxyConfig) (*ProxyServer, error) {
	if cfg == nil {
		return nil, fmt.Errorf("proxy config is required")
	}
	rc, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("in-cluster kube config: %w", err)
	}
	kube, err := kubernetes.NewForConfig(rc)
	if err != nil {
		return nil, fmt.Errorf("kubernetes client: %w", err)
	}

	var rdb *redis.Client
	if strings.TrimSpace(cfg.RedisURL) != "" {
		opt, err := redis.ParseURL(cfg.RedisURL)
		if err != nil {
			return nil, fmt.Errorf("parse redis URL: %w", err)
		}
		rdb = redis.NewClient(opt)
	}

	nsAllow := make(map[string]struct{}, len(cfg.AllowedNamespaces))
	for _, ns := range cfg.AllowedNamespaces {
		nsAllow[strings.TrimSpace(ns)] = struct{}{}
	}

	return &ProxyServer{
		cfg:            cfg,
		kube:           kube,
		redis:          rdb,
		namespaceAllow: nsAllow,
	}, nil
}

func (s *ProxyServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/k8s/status", s.requireAuth(s.handleK8sStatus))
	mux.HandleFunc("/k8s/metrics", s.requireAuth(s.handleK8sMetrics))
	mux.HandleFunc("/k8s/logs", s.requireAuth(s.handleK8sLogs))
	mux.HandleFunc("/redis/read", s.requireAuth(s.handleRedisRead))
	mux.HandleFunc("/redis/waitlist-emails", s.requireAuth(s.handleWaitlistEmails))
	return mux
}

func (s *ProxyServer) Close() error {
	if s == nil || s.redis == nil {
		return nil
	}
	return s.redis.Close()
}

func (s *ProxyServer) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer"))
		if subtleTrim(token) != subtleTrim(s.cfg.AuthToken) {
			log.Printf("ops_proxy: unauthorized path=%s", r.URL.Path)
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next(w, r)
	}
}

func (s *ProxyServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *ProxyServer) handleK8sStatus(w http.ResponseWriter, r *http.Request) {
	var req StatusRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Namespace = strings.TrimSpace(req.Namespace)
	if req.Namespace != "" && !s.namespaceAllowed(req.Namespace) {
		writeErr(w, http.StatusForbidden, "namespace not allowed")
		return
	}
	if req.Namespace == "" {
		req.Namespace = s.cfg.AllowedNamespaces[0]
	}
	limit := req.Limit
	if limit <= 0 {
		limit = s.cfg.DefaultStatusLimit
	}
	if limit > s.cfg.MaxStatusLimit {
		limit = s.cfg.MaxStatusLimit
	}
	log.Printf("ops_proxy: request path=/k8s/status namespace=%s target=%s limit=%d", req.Namespace, strings.TrimSpace(req.Target), limit)

	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.RequestTimeout)
	defer cancel()

	resp, err := s.readStatus(ctx, req.Namespace, strings.TrimSpace(req.Target), limit)
	if err != nil {
		log.Printf("ops_proxy: error path=/k8s/status err=%v", err)
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *ProxyServer) readStatus(ctx context.Context, namespace, target string, limit int) (StatusResponse, error) {
	out := StatusResponse{
		Namespace: namespace,
		Target:    target,
	}
	lowerTarget := strings.ToLower(strings.TrimSpace(target))
	switch {
	case strings.HasPrefix(lowerTarget, "deployment/"):
		name := strings.TrimSpace(strings.TrimPrefix(target, "deployment/"))
		dep, err := s.kube.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return StatusResponse{}, fmt.Errorf("get deployment: %w", err)
		}
		out.Deployments = []DeploymentStatus{mapDeployment(dep)}
		pods, err := s.podsForDeployment(ctx, namespace, dep, limit)
		if err == nil {
			out.Pods = mapPods(pods)
		}
		return out, nil
	case strings.HasPrefix(lowerTarget, "pod/"):
		name := strings.TrimSpace(strings.TrimPrefix(target, "pod/"))
		pod, err := s.kube.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return StatusResponse{}, fmt.Errorf("get pod: %w", err)
		}
		out.Pods = []PodStatus{mapPod(pod)}
		return out, nil
	}

	deps, err := s.kube.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return StatusResponse{}, fmt.Errorf("list deployments: %w", err)
	}
	sort.Slice(deps.Items, func(i, j int) bool { return deps.Items[i].Name < deps.Items[j].Name })
	for _, dep := range deps.Items {
		if len(out.Deployments) >= limit {
			break
		}
		out.Deployments = append(out.Deployments, mapDeployment(&dep))
	}

	pods, err := s.kube.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return StatusResponse{}, fmt.Errorf("list pods: %w", err)
	}
	sort.Slice(pods.Items, func(i, j int) bool { return pods.Items[i].Name < pods.Items[j].Name })
	for _, pod := range pods.Items {
		if len(out.Pods) >= limit {
			break
		}
		out.Pods = append(out.Pods, mapPod(&pod))
	}
	return out, nil
}

func (s *ProxyServer) handleK8sMetrics(w http.ResponseWriter, r *http.Request) {
	var req MetricsRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Namespace = strings.TrimSpace(req.Namespace)
	if req.Namespace != "" && !s.namespaceAllowed(req.Namespace) {
		writeErr(w, http.StatusForbidden, "namespace not allowed")
		return
	}
	if req.Namespace == "" {
		req.Namespace = s.cfg.AllowedNamespaces[0]
	}
	limit := req.Limit
	if limit <= 0 {
		limit = s.cfg.DefaultStatusLimit
	}
	if limit > s.cfg.MaxStatusLimit {
		limit = s.cfg.MaxStatusLimit
	}
	log.Printf("ops_proxy: request path=/k8s/metrics namespace=%s limit=%d", req.Namespace, limit)

	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.RequestTimeout)
	defer cancel()

	resp, err := s.readMetrics(ctx, req.Namespace, limit)
	if err != nil {
		log.Printf("ops_proxy: error path=/k8s/metrics err=%v", err)
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *ProxyServer) readMetrics(ctx context.Context, namespace string, limit int) (MetricsResponse, error) {
	nodes, err := s.kube.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return MetricsResponse{}, fmt.Errorf("list nodes: %w", err)
	}
	pods, err := s.kube.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return MetricsResponse{}, fmt.Errorf("list pods: %w", err)
	}

	usageByNode, usageErr := s.readNodeUsageMetrics(ctx)
	liveReason := ""
	if usageErr != nil {
		liveReason = usageErr.Error()
	}

	perNodeReq, perNodeLim := podResourceByNode(pods.Items)
	nodeRows := summarizeNodeResources(nodes.Items, perNodeReq, perNodeLim, usageByNode)

	cluster := ClusterResourceTotals{}
	for _, row := range nodeRows {
		cluster.CPUCapacityMilli += row.CPUCapacityMilli
		cluster.CPUAllocatableMilli += row.CPUAllocatableMilli
		cluster.CPURequestedMilli += row.CPURequestedMilli
		cluster.CPULimitsMilli += row.CPULimitsMilli
		cluster.CPUUsageMilli += row.CPUUsageMilli
		cluster.MemoryCapacityBytes += row.MemoryCapacityBytes
		cluster.MemoryAllocatableBytes += row.MemoryAllocatableBytes
		cluster.MemoryRequestedBytes += row.MemoryRequestedBytes
		cluster.MemoryLimitsBytes += row.MemoryLimitsBytes
		cluster.MemoryUsageBytes += row.MemoryUsageBytes
	}
	sort.Slice(nodeRows, func(i, j int) bool { return nodeRows[i].NodeName < nodeRows[j].NodeName })
	if len(nodeRows) > limit {
		nodeRows = nodeRows[:limit]
	}

	return MetricsResponse{
		Namespace:            namespace,
		LiveMetricsAvailable: usageErr == nil,
		LiveMetricsReason:    liveReason,
		Cluster:              cluster,
		Nodes:                nodeRows,
	}, nil
}

func podResourceByNode(pods []corev1.Pod) (map[string]corev1.ResourceList, map[string]corev1.ResourceList) {
	reqByNode := map[string]corev1.ResourceList{}
	limByNode := map[string]corev1.ResourceList{}
	for i := range pods {
		pod := pods[i]
		node := strings.TrimSpace(pod.Spec.NodeName)
		if node == "" {
			continue
		}
		if _, ok := reqByNode[node]; !ok {
			reqByNode[node] = corev1.ResourceList{}
			limByNode[node] = corev1.ResourceList{}
		}
		for _, ctr := range pod.Spec.Containers {
			addResource(reqByNode[node], corev1.ResourceCPU, ctr.Resources.Requests.Cpu())
			addResource(reqByNode[node], corev1.ResourceMemory, ctr.Resources.Requests.Memory())
			addResource(limByNode[node], corev1.ResourceCPU, ctr.Resources.Limits.Cpu())
			addResource(limByNode[node], corev1.ResourceMemory, ctr.Resources.Limits.Memory())
		}
	}
	return reqByNode, limByNode
}

func summarizeNodeResources(
	nodes []corev1.Node,
	reqByNode map[string]corev1.ResourceList,
	limByNode map[string]corev1.ResourceList,
	usageByNode map[string]corev1.ResourceList,
) []NodeResourceMetrics {
	out := make([]NodeResourceMetrics, 0, len(nodes))
	for i := range nodes {
		node := nodes[i]
		name := strings.TrimSpace(node.Name)
		req := reqByNode[name]
		lim := limByNode[name]
		usage := usageByNode[name]
		out = append(out, NodeResourceMetrics{
			NodeName:               name,
			CPUCapacityMilli:       quantityMilli(node.Status.Capacity.Cpu()),
			CPUAllocatableMilli:    quantityMilli(node.Status.Allocatable.Cpu()),
			CPURequestedMilli:      quantityMilli(req.Cpu()),
			CPULimitsMilli:         quantityMilli(lim.Cpu()),
			CPUUsageMilli:          quantityMilli(usage.Cpu()),
			MemoryCapacityBytes:    quantityBytes(node.Status.Capacity.Memory()),
			MemoryAllocatableBytes: quantityBytes(node.Status.Allocatable.Memory()),
			MemoryRequestedBytes:   quantityBytes(req.Memory()),
			MemoryLimitsBytes:      quantityBytes(lim.Memory()),
			MemoryUsageBytes:       quantityBytes(usage.Memory()),
		})
	}
	return out
}

func addResource(target corev1.ResourceList, key corev1.ResourceName, q *resource.Quantity) {
	if q == nil {
		return
	}
	current, ok := target[key]
	if !ok {
		target[key] = q.DeepCopy()
		return
	}
	current.Add(*q)
	target[key] = current
}

func quantityMilli(q *resource.Quantity) int64 {
	if q == nil {
		return 0
	}
	return q.MilliValue()
}

func quantityBytes(q *resource.Quantity) int64 {
	if q == nil {
		return 0
	}
	return q.Value()
}

func (s *ProxyServer) readNodeUsageMetrics(ctx context.Context) (map[string]corev1.ResourceList, error) {
	raw, err := s.kube.Discovery().RESTClient().Get().AbsPath("/apis/metrics.k8s.io/v1beta1/nodes").DoRaw(ctx)
	if err != nil {
		return nil, fmt.Errorf("metrics API unavailable: %w", err)
	}
	type nodeMetricsItem struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
		Usage map[string]string `json:"usage"`
	}
	type nodeMetricsList struct {
		Items []nodeMetricsItem `json:"items"`
	}
	var parsed nodeMetricsList
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("decode metrics API response: %w", err)
	}
	usageByNode := make(map[string]corev1.ResourceList, len(parsed.Items))
	for _, item := range parsed.Items {
		nodeName := strings.TrimSpace(item.Metadata.Name)
		if nodeName == "" {
			continue
		}
		nodeUsage := corev1.ResourceList{}
		for rk, rv := range item.Usage {
			key := corev1.ResourceName(strings.TrimSpace(rk))
			q, err := resource.ParseQuantity(strings.TrimSpace(rv))
			if err != nil {
				continue
			}
			nodeUsage[key] = q
		}
		usageByNode[nodeName] = nodeUsage
	}
	return usageByNode, nil
}

func (s *ProxyServer) podsForDeployment(ctx context.Context, namespace string, dep *appsv1.Deployment, limit int) ([]corev1.Pod, error) {
	if dep == nil || dep.Spec.Selector == nil {
		return nil, fmt.Errorf("deployment has no selector")
	}
	selector, err := metav1.LabelSelectorAsSelector(dep.Spec.Selector)
	if err != nil {
		return nil, fmt.Errorf("selector: %w", err)
	}
	pods, err := s.kube.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(pods.Items, func(i, j int) bool { return pods.Items[i].Name < pods.Items[j].Name })
	if len(pods.Items) > limit {
		return pods.Items[:limit], nil
	}
	return pods.Items, nil
}

func (s *ProxyServer) handleK8sLogs(w http.ResponseWriter, r *http.Request) {
	var req LogsRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	ns := strings.TrimSpace(req.Namespace)
	if ns == "" {
		ns = s.cfg.AllowedNamespaces[0]
	}
	if !s.namespaceAllowed(ns) {
		writeErr(w, http.StatusForbidden, "namespace not allowed")
		return
	}
	target := strings.TrimSpace(req.Target)
	if target == "" {
		writeErr(w, http.StatusBadRequest, "target is required")
		return
	}
	tail := req.TailLines
	if tail <= 0 {
		tail = s.cfg.DefaultLogTailLines
	}
	if tail > s.cfg.MaxLogTailLines {
		tail = s.cfg.MaxLogTailLines
	}
	since := req.SinceSeconds
	if since < 0 {
		since = 0
	}
	log.Printf("ops_proxy: request path=/k8s/logs namespace=%s target=%s tail=%d since=%d", ns, target, tail, since)

	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.RequestTimeout)
	defer cancel()
	lines, resolvedTarget, truncated, err := s.readLogs(ctx, ns, target, strings.TrimSpace(req.Container), tail, since)
	if err != nil {
		log.Printf("ops_proxy: error path=/k8s/logs err=%v", err)
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, LogsResponse{
		Namespace: ns,
		Target:    resolvedTarget,
		Lines:     lines,
		Truncated: truncated,
	})
}

func (s *ProxyServer) readLogs(ctx context.Context, namespace, target, container string, tailLines, sinceSeconds int64) (string, string, bool, error) {
	lower := strings.ToLower(target)
	var podName string
	switch {
	case strings.HasPrefix(lower, "pod/"):
		podName = strings.TrimSpace(strings.TrimPrefix(target, "pod/"))
	case strings.HasPrefix(lower, "deployment/"):
		depName := strings.TrimSpace(strings.TrimPrefix(target, "deployment/"))
		dep, err := s.kube.AppsV1().Deployments(namespace).Get(ctx, depName, metav1.GetOptions{})
		if err != nil {
			return "", "", false, fmt.Errorf("get deployment: %w", err)
		}
		pods, err := s.podsForDeployment(ctx, namespace, dep, 1)
		if err != nil || len(pods) == 0 {
			return "", "", false, fmt.Errorf("resolve deployment pod: %w", err)
		}
		podName = pods[0].Name
	default:
		return "", "", false, fmt.Errorf("target must be pod/<name> or deployment/<name>")
	}

	opts := &corev1.PodLogOptions{
		Container:    container,
		TailLines:    &tailLines,
		SinceSeconds: &sinceSeconds,
	}
	stream, err := s.kube.CoreV1().Pods(namespace).GetLogs(podName, opts).Stream(ctx)
	if err != nil {
		return "", "", false, fmt.Errorf("open logs stream: %w", err)
	}
	defer stream.Close()
	raw, err := io.ReadAll(io.LimitReader(stream, int64(s.cfg.MaxLogBytes)+1))
	if err != nil {
		return "", "", false, fmt.Errorf("read logs: %w", err)
	}
	truncated := len(raw) > s.cfg.MaxLogBytes
	if truncated {
		raw = raw[:s.cfg.MaxLogBytes]
	}
	return strings.TrimSpace(string(raw)), "pod/" + podName, truncated, nil
}

func (s *ProxyServer) handleRedisRead(w http.ResponseWriter, r *http.Request) {
	if s.redis == nil {
		writeErr(w, http.StatusServiceUnavailable, "redis is not configured")
		return
	}
	var req RedisReadRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	key := strings.TrimSpace(req.Key)
	prefix := strings.TrimSpace(req.Prefix)
	if key == "" && prefix == "" {
		writeErr(w, http.StatusBadRequest, "key or prefix is required")
		return
	}
	limit := req.Limit
	if limit <= 0 {
		limit = s.cfg.DefaultRedisLimit
	}
	if limit > s.cfg.MaxRedisLimit {
		limit = s.cfg.MaxRedisLimit
	}
	log.Printf("ops_proxy: request path=/redis/read key=%s prefix=%s limit=%d", key, prefix, limit)

	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.RequestTimeout)
	defer cancel()

	resp, err := s.readRedis(ctx, key, prefix, limit)
	if err != nil {
		log.Printf("ops_proxy: error path=/redis/read err=%v", err)
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *ProxyServer) handleWaitlistEmails(w http.ResponseWriter, r *http.Request) {
	if s.redis == nil {
		writeErr(w, http.StatusServiceUnavailable, "redis is not configured")
		return
	}
	var req WaitlistEmailsRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	prefixes, err := s.resolveWaitlistPrefixes(strings.TrimSpace(req.Prefix))
	if err != nil {
		writeErr(w, http.StatusForbidden, err.Error())
		return
	}
	limit := req.Limit
	if limit <= 0 {
		limit = s.cfg.DefaultWaitlistLimit
	}
	if limit > s.cfg.MaxWaitlistLimit {
		limit = s.cfg.MaxWaitlistLimit
	}
	log.Printf(
		"ops_proxy: request path=/redis/waitlist-emails prefixes=%s limit=%d reveal_full=%t",
		strings.Join(prefixes, ","),
		limit,
		req.RevealFull,
	)
	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.RequestTimeout)
	defer cancel()
	resp, err := s.readWaitlistEmails(ctx, prefixes, limit, req.RevealFull)
	if err != nil {
		log.Printf("ops_proxy: error path=/redis/waitlist-emails err=%v", err)
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *ProxyServer) readRedis(ctx context.Context, key, prefix string, limit int) (RedisReadResponse, error) {
	if key != "" {
		if !s.redisPrefixAllowed(key) {
			return RedisReadResponse{}, fmt.Errorf("redis key is not allowed")
		}
		item, err := s.readRedisKey(ctx, key)
		if err != nil {
			return RedisReadResponse{}, err
		}
		return RedisReadResponse{Items: []RedisItem{item}}, nil
	}
	if !s.redisPrefixAllowed(prefix) {
		return RedisReadResponse{}, fmt.Errorf("redis prefix is not allowed")
	}
	match := prefix + "*"
	var cursor uint64
	items := make([]RedisItem, 0, limit)
	for len(items) < limit {
		keys, next, err := s.redis.Scan(ctx, cursor, match, int64(limit-len(items))).Result()
		if err != nil {
			return RedisReadResponse{}, fmt.Errorf("redis scan: %w", err)
		}
		for _, k := range keys {
			item, err := s.readRedisKey(ctx, k)
			if err != nil {
				continue
			}
			items = append(items, item)
			if len(items) >= limit {
				break
			}
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return RedisReadResponse{Items: items}, nil
}

var emailRegex = regexp.MustCompile(`(?i)\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}\b`)

func (s *ProxyServer) readWaitlistEmails(ctx context.Context, prefixes []string, limit int, revealFull bool) (WaitlistEmailsResponse, error) {
	if len(prefixes) == 0 {
		return WaitlistEmailsResponse{}, fmt.Errorf("no waitlist prefixes configured")
	}
	seen := map[string]struct{}{}
	out := make([]WaitlistEmail, 0, limit)
	for _, prefix := range prefixes {
		match := prefix + "*"
		var cursor uint64
		for len(out) < limit {
			keys, next, err := s.redis.Scan(ctx, cursor, match, int64(limit*3)).Result()
			if err != nil {
				return WaitlistEmailsResponse{}, fmt.Errorf("redis scan: %w", err)
			}
			for _, key := range keys {
				item, err := s.readRedisKey(ctx, key)
				if err != nil {
					continue
				}
				found := emailRegex.FindAllString(item.Value, -1)
				for _, email := range found {
					normalized := strings.ToLower(strings.TrimSpace(email))
					if normalized == "" {
						continue
					}
					if _, ok := seen[normalized]; ok {
						continue
					}
					seen[normalized] = struct{}{}
					value := normalized
					if !revealFull {
						value = maskEmail(normalized)
					}
					out = append(out, WaitlistEmail{
						Email:  value,
						Source: key,
					})
					if len(out) >= limit {
						break
					}
				}
				if len(out) >= limit {
					break
				}
			}
			cursor = next
			if cursor == 0 {
				break
			}
		}
		if len(out) >= limit {
			break
		}
	}
	return WaitlistEmailsResponse{
		Emails:           out,
		SearchedPrefixes: prefixes,
	}, nil
}

func (s *ProxyServer) resolveWaitlistPrefixes(requestedPrefix string) ([]string, error) {
	req := strings.TrimSpace(requestedPrefix)
	if req != "" {
		if !s.waitlistPrefixAllowed(req) {
			return nil, fmt.Errorf("waitlist prefix is not allowed")
		}
		return []string{req}, nil
	}
	return prioritizeWaitlistPrefixes(s.cfg.WaitlistPrefixes), nil
}

func prioritizeWaitlistPrefixes(prefixes []string) []string {
	if len(prefixes) == 0 {
		return nil
	}
	out := make([]string, 0, len(prefixes))
	seen := map[string]struct{}{}

	// Prefer the canonical makeacompany waitlist keyspace first.
	for _, p := range prefixes {
		v := strings.TrimSpace(p)
		if v == "" {
			continue
		}
		if strings.HasPrefix(v, "makeacompany:waitlist:") {
			if _, ok := seen[v]; ok {
				continue
			}
			out = append(out, v)
			seen[v] = struct{}{}
		}
	}
	for _, p := range prefixes {
		v := strings.TrimSpace(p)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		out = append(out, v)
		seen[v] = struct{}{}
	}
	return out
}

func (s *ProxyServer) readRedisKey(ctx context.Context, key string) (RedisItem, error) {
	typ, err := s.redis.Type(ctx, key).Result()
	if err != nil {
		return RedisItem{}, fmt.Errorf("redis type: %w", err)
	}
	var value string
	switch typ {
	case "string":
		value, err = s.redis.Get(ctx, key).Result()
	case "hash":
		m, herr := s.redis.HGetAll(ctx, key).Result()
		if herr != nil {
			err = herr
		} else {
			b, _ := json.Marshal(m)
			value = string(b)
		}
	case "list":
		items, lerr := s.redis.LRange(ctx, key, 0, 20).Result()
		if lerr != nil {
			err = lerr
		} else {
			b, _ := json.Marshal(items)
			value = string(b)
		}
	case "set":
		items, serr := s.redis.SMembers(ctx, key).Result()
		if serr != nil {
			err = serr
		} else {
			b, _ := json.Marshal(items)
			value = string(b)
		}
	case "zset":
		items, zerr := s.redis.ZRangeWithScores(ctx, key, 0, 20).Result()
		if zerr != nil {
			err = zerr
		} else {
			b, _ := json.Marshal(items)
			value = string(b)
		}
	default:
		value = "<unsupported_type:" + typ + ">"
	}
	if err != nil {
		return RedisItem{}, fmt.Errorf("redis read: %w", err)
	}
	return RedisItem{
		Key:   key,
		Type:  typ,
		Value: value,
	}, nil
}

func mapDeployment(dep *appsv1.Deployment) DeploymentStatus {
	if dep == nil {
		return DeploymentStatus{}
	}
	images := make([]string, 0, len(dep.Spec.Template.Spec.Containers))
	for _, c := range dep.Spec.Template.Spec.Containers {
		images = append(images, c.Image)
	}
	return DeploymentStatus{
		Namespace:         dep.Namespace,
		Name:              dep.Name,
		Replicas:          dep.Status.Replicas,
		ReadyReplicas:     dep.Status.ReadyReplicas,
		UpdatedReplicas:   dep.Status.UpdatedReplicas,
		AvailableReplicas: dep.Status.AvailableReplicas,
		Images:            images,
	}
}

func mapPods(pods []corev1.Pod) []PodStatus {
	out := make([]PodStatus, 0, len(pods))
	for i := range pods {
		out = append(out, mapPod(&pods[i]))
	}
	return out
}

func mapPod(pod *corev1.Pod) PodStatus {
	if pod == nil {
		return PodStatus{}
	}
	ready := false
	var restarts int32
	for _, c := range pod.Status.ContainerStatuses {
		if c.Ready {
			ready = true
		}
		restarts += c.RestartCount
	}
	images := make([]string, 0, len(pod.Spec.Containers))
	for _, c := range pod.Spec.Containers {
		images = append(images, c.Image)
	}
	return PodStatus{
		Namespace: pod.Namespace,
		Name:      pod.Name,
		Phase:     string(pod.Status.Phase),
		Ready:     ready,
		Restarts:  restarts,
		NodeName:  pod.Spec.NodeName,
		Images:    images,
	}
}

func (s *ProxyServer) namespaceAllowed(ns string) bool {
	_, ok := s.namespaceAllow[strings.TrimSpace(ns)]
	return ok
}

func (s *ProxyServer) redisPrefixAllowed(v string) bool {
	value := strings.TrimSpace(v)
	if value == "" {
		return false
	}
	for _, prefix := range s.cfg.AllowedRedisPrefixes {
		p := strings.TrimSpace(prefix)
		if p == "" {
			continue
		}
		if strings.HasPrefix(value, p) {
			return true
		}
	}
	return false
}

func (s *ProxyServer) waitlistPrefixAllowed(v string) bool {
	value := strings.TrimSpace(v)
	if value == "" {
		return false
	}
	for _, prefix := range s.cfg.WaitlistPrefixes {
		p := strings.TrimSpace(prefix)
		if p == "" {
			continue
		}
		if strings.HasPrefix(value, p) {
			return true
		}
	}
	return false
}

func decodeJSONBody(r *http.Request, out any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 256*1024))
	if err != nil {
		return fmt.Errorf("read request: %w", err)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("invalid JSON body")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("ops_proxy: write response: %v", err)
	}
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": strings.TrimSpace(msg)})
}

func subtleTrim(v string) string {
	return strings.TrimSpace(v)
}

func maskEmail(email string) string {
	parts := strings.Split(strings.TrimSpace(email), "@")
	if len(parts) != 2 {
		return email
	}
	local := parts[0]
	domain := parts[1]
	if len(local) == 0 {
		return "***@" + domain
	}
	if len(local) <= 2 {
		return local[:1] + "***@" + domain
	}
	return local[:1] + "***" + local[len(local)-1:] + "@" + domain
}
