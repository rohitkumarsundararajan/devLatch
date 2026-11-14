package infer

import (
	"sort"
	"sync"
)

// EndpointSchema is a point-in-time copy of what was learned for one endpoint.
type EndpointSchema struct {
	Key       string // e.g. "GET /api/v1/users/{id} 200"
	Responses int    // payloads merged into Node
	Node      *Node
}

// Store accumulates the structure observed per endpoint. It is safe for
// concurrent use; each endpoint has its own lock so workers rarely contend.
type Store struct {
	inf *Inferrer
	mu  sync.RWMutex
	eps map[string]*endpoint
}

type endpoint struct {
	mu        sync.Mutex
	node      *Node
	responses int
}

// NewStore returns an empty Store that infers with cfg.
func NewStore(cfg Config) *Store {
	return &Store{inf: New(cfg), eps: map[string]*endpoint{}}
}

// Observe infers the structure of body and merges it into endpoint key.
// Inference happens outside any lock.
func (s *Store) Observe(key string, body []byte) error {
	n, err := s.inf.Observe(body)
	if err != nil {
		return err
	}
	ep := s.endpoint(key)
	ep.mu.Lock()
	ep.node = s.inf.Merge(ep.node, n)
	ep.responses++
	ep.mu.Unlock()
	return nil
}

func (s *Store) endpoint(key string) *endpoint {
	s.mu.RLock()
	ep, ok := s.eps[key]
	s.mu.RUnlock()
	if ok {
		return ep
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if ep, ok = s.eps[key]; !ok {
		ep = &endpoint{}
		s.eps[key] = ep
	}
	return ep
}

// Keys returns the endpoint keys seen so far, sorted.
func (s *Store) Keys() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	keys := make([]string, 0, len(s.eps))
	for k, ep := range s.eps {
		ep.mu.Lock()
		seen := ep.node != nil
		ep.mu.Unlock()
		if seen {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

// Snapshot returns deep copies of every endpoint's schema, sorted by key.
func (s *Store) Snapshot() []EndpointSchema {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]EndpointSchema, 0, len(s.eps))
	for k, ep := range s.eps {
		ep.mu.Lock()
		if ep.node != nil {
			out = append(out, EndpointSchema{Key: k, Responses: ep.responses, Node: ep.node.Clone()})
		}
		ep.mu.Unlock()
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}
