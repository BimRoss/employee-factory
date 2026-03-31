package persona

import (
	"os"
	"strings"
	"sync"
	"time"
)

// Loader reads the persona markdown from disk and optionally reloads on an interval.
type Loader struct {
	path     string
	interval time.Duration

	mu      sync.RWMutex
	content string
}

// NewLoader returns a loader; if reload > 0, StartBackgroundReload should be called.
func NewLoader(path string, reloadMS int) *Loader {
	return &Loader{
		path:     path,
		interval: time.Duration(reloadMS) * time.Millisecond,
	}
}

// Load reads the file once into memory.
func (l *Loader) Load() error {
	b, err := os.ReadFile(l.path)
	if err != nil {
		return err
	}
	l.mu.Lock()
	l.content = strings.TrimSpace(string(b))
	l.mu.Unlock()
	return nil
}

// String returns the current persona text (may be empty if not loaded).
func (l *Loader) String() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.content
}

// StartBackgroundReload polls the file for changes on interval (best-effort).
func (l *Loader) StartBackgroundReload() {
	if l.interval <= 0 {
		return
	}
	go func() {
		t := time.NewTicker(l.interval)
		defer t.Stop()
		for range t.C {
			_ = l.Load()
		}
	}()
}
