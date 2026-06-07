package proxy

import (
	"fmt"
	"math/rand"
	"net/url"
	"sync"
	"time"

	"github.com/ashleyfullero/scrapeowl/internal/config"
)

// Proxy represents a single proxy entry
type Proxy struct {
	URL      string
	parsed   *url.URL
	Failures int
	LastUsed time.Time
	Healthy  bool
}

// Pool manages a collection of proxies with rotation support
type Pool struct {
	mu       sync.RWMutex
	proxies  []*Proxy
	index    int
	cfg      config.ProxyConfig
	rotType  RotationType
}

// RotationType defines how proxies are selected
type RotationType int

const (
	RotationRoundRobin RotationType = iota
	RotationRandom
)

// MaxFailures is how many failures before a proxy is marked unhealthy
const MaxFailures = 3

// NewPool creates a proxy pool from the configuration
func NewPool(cfg config.ProxyConfig) (*Pool, error) {
	p := &Pool{
		cfg:     cfg,
		rotType: RotationRoundRobin,
	}

	switch cfg.Type {
	case "none", "":
		// No proxies — direct connections
		return p, nil
	case "static", "rotating":
		for _, rawURL := range cfg.List {
			px, err := parseProxy(rawURL)
			if err != nil {
				return nil, fmt.Errorf("invalid proxy URL '%s': %w", rawURL, err)
			}
			p.proxies = append(p.proxies, px)
		}
		if cfg.Type == "rotating" {
			p.rotType = RotationRandom
		}
	default:
		return nil, fmt.Errorf("unknown proxy type: %s", cfg.Type)
	}

	return p, nil
}

func parseProxy(rawURL string) (*Proxy, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	return &Proxy{
		URL:     rawURL,
		parsed:  u,
		Healthy: true,
	}, nil
}

// Next returns the next healthy proxy URL, or empty string if no proxies configured
func (p *Pool) Next() string {
	p.mu.Lock()
	defer p.mu.Unlock()

	healthy := p.healthyProxies()
	if len(healthy) == 0 {
		return ""
	}

	var selected *Proxy
	switch p.rotType {
	case RotationRandom:
		selected = healthy[rand.Intn(len(healthy))]
	default: // RoundRobin
		p.index = p.index % len(healthy)
		selected = healthy[p.index]
		p.index++
	}

	selected.LastUsed = time.Now()
	return selected.URL
}

// MarkFailure increments failure count for a proxy and may mark it unhealthy
func (p *Pool) MarkFailure(proxyURL string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, px := range p.proxies {
		if px.URL == proxyURL {
			px.Failures++
			if px.Failures >= MaxFailures {
				px.Healthy = false
			}
			return
		}
	}
}

// MarkSuccess resets failure count for a proxy
func (p *Pool) MarkSuccess(proxyURL string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, px := range p.proxies {
		if px.URL == proxyURL {
			px.Failures = 0
			px.Healthy = true
			return
		}
	}
}

// healthyProxies returns all healthy proxies (must be called with lock held)
func (p *Pool) healthyProxies() []*Proxy {
	var healthy []*Proxy
	for _, px := range p.proxies {
		if px.Healthy {
			healthy = append(healthy, px)
		}
	}
	// If all proxies are unhealthy, reset and try again
	if len(healthy) == 0 && len(p.proxies) > 0 {
		for _, px := range p.proxies {
			px.Healthy = true
			px.Failures = 0
		}
		healthy = p.proxies
	}
	return healthy
}

// Stats returns a snapshot of the pool status
func (p *Pool) Stats() PoolStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	total := len(p.proxies)
	healthy := 0
	for _, px := range p.proxies {
		if px.Healthy {
			healthy++
		}
	}
	return PoolStats{
		Total:     total,
		Healthy:   healthy,
		Unhealthy: total - healthy,
		ProxyType: p.cfg.Type,
	}
}

// List returns a copy of all proxies for display
func (p *Pool) List() []ProxyStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()

	out := make([]ProxyStatus, len(p.proxies))
	for i, px := range p.proxies {
		// Redact password in URL for display
		displayURL := px.URL
		if px.parsed != nil && px.parsed.User != nil {
			redacted := *px.parsed
			redacted.User = url.UserPassword(px.parsed.User.Username(), "***")
			displayURL = redacted.String()
		}
		out[i] = ProxyStatus{
			URL:      displayURL,
			Healthy:  px.Healthy,
			Failures: px.Failures,
			LastUsed: px.LastUsed,
		}
	}
	return out
}

// PoolStats summarizes the state of the proxy pool
type PoolStats struct {
	Total     int    `json:"total"`
	Healthy   int    `json:"healthy"`
	Unhealthy int    `json:"unhealthy"`
	ProxyType string `json:"proxy_type"`
}

// ProxyStatus represents a single proxy's display status
type ProxyStatus struct {
	URL      string    `json:"url"`
	Healthy  bool      `json:"healthy"`
	Failures int       `json:"failures"`
	LastUsed time.Time `json:"last_used"`
}
