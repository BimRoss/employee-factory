package opsproxy

import (
	"encoding/json"
	"sort"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRedisPrefixAllowed(t *testing.T) {
	s := &ProxyServer{
		cfg: &ProxyConfig{
			AllowedRedisPrefixes: []string{"thread_owner:", "ops:"},
			WaitlistPrefixes:     []string{"waitlist:"},
		},
	}
	if !s.redisPrefixAllowed("thread_owner:C123") {
		t.Fatal("expected thread_owner prefix to be allowed")
	}
	if s.redisPrefixAllowed("other:key") {
		t.Fatal("did not expect unrelated prefix to be allowed")
	}
	if !s.waitlistPrefixAllowed("waitlist:user:1") {
		t.Fatal("expected waitlist prefix to be allowed")
	}
	if s.waitlistPrefixAllowed("thread_owner:C123") {
		t.Fatal("did not expect thread_owner key in waitlist prefix set")
	}
}

func TestResolveWaitlistPrefixes_DefaultPrioritizesMakeACompany(t *testing.T) {
	s := &ProxyServer{
		cfg: &ProxyConfig{
			WaitlistPrefixes: []string{"waitlist:", "makeacompany:waitlist:", "legacy:waitlist:"},
		},
	}
	prefixes, err := s.resolveWaitlistPrefixes("")
	if err != nil {
		t.Fatalf("resolve default prefixes: %v", err)
	}
	if len(prefixes) != 3 {
		t.Fatalf("expected 3 prefixes, got %d", len(prefixes))
	}
	if prefixes[0] != "makeacompany:waitlist:" {
		t.Fatalf("expected makeacompany prefix first, got %q", prefixes[0])
	}
}

func TestResolveWaitlistPrefixes_ExplicitAllowed(t *testing.T) {
	s := &ProxyServer{
		cfg: &ProxyConfig{
			WaitlistPrefixes: []string{"waitlist:", "makeacompany:waitlist:"},
		},
	}
	prefixes, err := s.resolveWaitlistPrefixes("waitlist:")
	if err != nil {
		t.Fatalf("resolve explicit prefix: %v", err)
	}
	if len(prefixes) != 1 || prefixes[0] != "waitlist:" {
		t.Fatalf("unexpected explicit prefixes result: %#v", prefixes)
	}
}

func TestPodResourceByNode(t *testing.T) {
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "p1"},
			Spec: corev1.PodSpec{
				NodeName: "node-a",
				Containers: []corev1.Container{
					{
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("250m"),
								corev1.ResourceMemory: resource.MustParse("256Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("500m"),
								corev1.ResourceMemory: resource.MustParse("512Mi"),
							},
						},
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "p2"},
			Spec: corev1.PodSpec{
				NodeName: "node-a",
				Containers: []corev1.Container{
					{
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse("100m"),
							},
						},
					},
				},
			},
		},
	}
	reqByNode, limByNode := podResourceByNode(pods)
	reqCPU := reqByNode["node-a"][corev1.ResourceCPU]
	if got := (&reqCPU).MilliValue(); got != 350 {
		t.Fatalf("cpu requests mismatch: got=%d want=350", got)
	}
	limCPU := limByNode["node-a"][corev1.ResourceCPU]
	if got := (&limCPU).MilliValue(); got != 500 {
		t.Fatalf("cpu limits mismatch: got=%d want=500", got)
	}
}

func TestSummarizeNodeResources(t *testing.T) {
	nodes := []corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
			Status: corev1.NodeStatus{
				Capacity: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("3800m"),
					corev1.ResourceMemory: resource.MustParse("7600Mi"),
				},
			},
		},
	}
	reqByNode := map[string]corev1.ResourceList{
		"node-a": {
			corev1.ResourceCPU:    resource.MustParse("1200m"),
			corev1.ResourceMemory: resource.MustParse("2Gi"),
		},
	}
	limByNode := map[string]corev1.ResourceList{
		"node-a": {
			corev1.ResourceCPU:    resource.MustParse("2400m"),
			corev1.ResourceMemory: resource.MustParse("4Gi"),
		},
	}
	usageByNode := map[string]corev1.ResourceList{
		"node-a": {
			corev1.ResourceCPU:    resource.MustParse("800m"),
			corev1.ResourceMemory: resource.MustParse("1536Mi"),
		},
	}
	rows := summarizeNodeResources(nodes, reqByNode, limByNode, usageByNode)
	if len(rows) != 1 {
		t.Fatalf("expected one node row, got %d", len(rows))
	}
	if rows[0].CPUUsageMilli != 800 {
		t.Fatalf("cpu usage mismatch: got=%d want=800", rows[0].CPUUsageMilli)
	}
	if rows[0].CPURequestedMilli != 1200 {
		t.Fatalf("cpu requested mismatch: got=%d want=1200", rows[0].CPURequestedMilli)
	}
	if rows[0].MemoryUsageBytes <= 0 {
		t.Fatalf("expected memory usage bytes > 0")
	}
}

func TestExtractWaitlistCandidates_HashWithEmail(t *testing.T) {
	values := map[string]string{
		"email":     "User@Example.com",
		"updatedAt": "2026-04-04T12:00:00Z",
	}
	raw, err := json.Marshal(values)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := extractWaitlistCandidates(RedisItem{
		Type:  "hash",
		Value: string(raw),
	})
	if len(got) != 1 {
		t.Fatalf("expected one candidate, got %d", len(got))
	}
	if got[0].Email != "user@example.com" {
		t.Fatalf("email mismatch: got=%q", got[0].Email)
	}
	if got[0].UpdatedAt != "2026-04-04T12:00:00Z" {
		t.Fatalf("updatedAt mismatch: got=%q", got[0].UpdatedAt)
	}
}

func TestExtractWaitlistCandidates_FallbackRegex(t *testing.T) {
	got := extractWaitlistCandidates(RedisItem{
		Type:  "string",
		Value: `{"email":"fallback@example.com"}`,
	})
	if len(got) != 1 {
		t.Fatalf("expected one candidate, got %d", len(got))
	}
	if got[0].Email != "fallback@example.com" {
		t.Fatalf("email mismatch: got=%q", got[0].Email)
	}
	if got[0].UpdatedAt != "" {
		t.Fatalf("expected empty updatedAt, got %q", got[0].UpdatedAt)
	}
}

func TestWaitlistEmailIsNewer_PrefersTimestamp(t *testing.T) {
	rows := []WaitlistEmail{
		{
			Email:     "second@example.com",
			UpdatedAt: "2026-04-04T10:00:00Z",
			SourceKey: "makeacompany:waitlist:second@example.com",
		},
		{
			Email:     "first@example.com",
			UpdatedAt: "2026-04-04T11:00:00Z",
			SourceKey: "makeacompany:waitlist:first@example.com",
		},
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return waitlistEmailIsNewer(rows[i], rows[j])
	})
	if rows[0].Email != "first@example.com" {
		t.Fatalf("expected newest timestamp first, got %q", rows[0].Email)
	}
}

func TestWaitlistEmailIsNewer_TimestampBeatsMissing(t *testing.T) {
	rows := []WaitlistEmail{
		{
			Email:     "missing@example.com",
			UpdatedAt: "",
			SourceKey: "makeacompany:waitlist:missing@example.com",
		},
		{
			Email:     "has@example.com",
			UpdatedAt: "2026-04-04T11:00:00Z",
			SourceKey: "makeacompany:waitlist:has@example.com",
		},
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return waitlistEmailIsNewer(rows[i], rows[j])
	})
	if rows[0].Email != "has@example.com" {
		t.Fatalf("expected row with timestamp first, got %q", rows[0].Email)
	}
}

func TestParseWaitlistUpdatedAt(t *testing.T) {
	parsed, ok := parseWaitlistUpdatedAt("2026-04-04T11:00:00Z")
	if !ok {
		t.Fatal("expected timestamp parse to succeed")
	}
	if parsed.UTC().Format(time.RFC3339) != "2026-04-04T11:00:00Z" {
		t.Fatalf("unexpected parse value: %s", parsed.UTC().Format(time.RFC3339))
	}
	if _, ok := parseWaitlistUpdatedAt("not-a-date"); ok {
		t.Fatal("expected invalid timestamp parse to fail")
	}
}
