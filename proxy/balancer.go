package proxy

import (
	"math/rand/v2"
	"sync"
	"sync/atomic"
)

// Algorithm selects which target receives the next request or connection.
type Algorithm string

const (
	// RoundRobin cycles through the healthy targets in order.
	RoundRobin Algorithm = "round_robin"
	// Random picks a healthy target uniformly at random.
	Random Algorithm = "random"
	// LeastConn picks the healthy target with the fewest in-flight requests.
	LeastConn Algorithm = "least_conn"
	// IPHash sends the same client IP to the same healthy target, so a target
	// keeps serving a given client for as long as it stays healthy.
	IPHash Algorithm = "ip_hash"
)

// Target is one upstream server behind the proxy.
type Target struct {
	// URL is the upstream base URL for an HTTP proxy ("http://10.0.0.1:8080"),
	// or the "host:port" address for a TCP proxy.
	URL string
	// Weight biases selection toward this target: a target with Weight 3 is
	// chosen three times as often as one with Weight 1. Zero means 1.
	Weight int

	healthy atomic.Bool
	active  atomic.Int64
	fails   atomic.Int64
}

// Healthy reports whether the health checker currently considers the target
// usable. Targets start healthy so a proxy works before the first probe.
func (t *Target) Healthy() bool { return t.healthy.Load() }

// Active returns the number of requests or connections currently in flight to
// this target.
func (t *Target) Active() int64 { return t.active.Load() }

func (t *Target) weight() int {
	if t.Weight <= 0 {
		return 1
	}
	return t.Weight
}

// Balancer holds the target pool and picks one target per request. It is safe
// for concurrent use.
type Balancer struct {
	targets []*Target
	algo    Algorithm
	counter atomic.Uint64
	mu      sync.RWMutex
}

// NewBalancer returns a Balancer over targets. Every target starts healthy;
// an unknown algorithm falls back to round robin.
func NewBalancer(targets []*Target, algo Algorithm) *Balancer {
	switch algo {
	case RoundRobin, Random, LeastConn, IPHash:
	default:
		algo = RoundRobin
	}
	for _, t := range targets {
		t.healthy.Store(true)
	}
	return &Balancer{targets: targets, algo: algo}
}

// Targets returns the target pool.
func (b *Balancer) Targets() []*Target {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.targets
}

// Pick returns a healthy target, or nil when every target is down. key is the
// client identity used by IPHash and ignored by the other algorithms.
func (b *Balancer) Pick(key string) *Target {
	b.mu.RLock()
	defer b.mu.RUnlock()

	// Expand by weight so a heavier target simply appears more than once; the
	// pool is small, so this is cheaper than tracking smooth-weighted state.
	pool := make([]*Target, 0, len(b.targets))
	for _, t := range b.targets {
		if !t.healthy.Load() {
			continue
		}
		for i := 0; i < t.weight(); i++ {
			pool = append(pool, t)
		}
	}
	if len(pool) == 0 {
		return nil
	}

	switch b.algo {
	case Random:
		return pool[rand.IntN(len(pool))]
	case LeastConn:
		best := pool[0]
		for _, t := range pool[1:] {
			if t.active.Load() < best.active.Load() {
				best = t
			}
		}
		return best
	case IPHash:
		return pool[int(fnv1a(key)%uint64(len(pool)))]
	default:
		n := b.counter.Add(1) - 1
		return pool[n%uint64(len(pool))]
	}
}

// fnv1a is the 64-bit FNV-1a hash, used to map a client key onto a target.
func fnv1a(s string) uint64 {
	const (
		offset = 14695981039346656037
		prime  = 1099511628211
	)
	h := uint64(offset)
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= prime
	}
	return h
}
