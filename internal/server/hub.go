// Package server is the dashboard backend: a read-only JSON API plus a
// Server-Sent Events stream of live drift events. It binds to localhost only
// (the caller picks the listener) and rejects foreign Host headers.
package server

import (
	"encoding/json"
	"sync"
	"time"
)

// Event is one item on the live stream.
type Event struct {
	ID   int64           `json:"id"`
	Type string          `json:"type"` // "drift", "endpoint", "status"
	Time time.Time       `json:"time"`
	Data json.RawMessage `json:"data"`
}

// Hub fans events out to subscribers and keeps a bounded history so a
// reconnecting client can catch up with Last-Event-ID.
type Hub struct {
	mu     sync.Mutex
	max    int
	nextID int64
	ring   []Event
	subs   map[chan Event]struct{}
	closed bool
}

func NewHub(max int) *Hub {
	if max <= 0 {
		max = 500
	}
	return &Hub{max: max, subs: map[chan Event]struct{}{}}
}

// Publish stores and broadcasts an event. A subscriber whose buffer is full
// misses it (it can recover through replay on reconnect); Publish never blocks.
func (h *Hub) Publish(typ string, data any) Event {
	raw, err := json.Marshal(data)
	if err != nil {
		raw = []byte("null")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return Event{}
	}
	h.nextID++
	ev := Event{ID: h.nextID, Type: typ, Time: time.Now().UTC(), Data: raw}
	h.ring = append(h.ring, ev)
	if len(h.ring) > h.max {
		h.ring = append([]Event(nil), h.ring[len(h.ring)-h.max:]...)
	}
	for ch := range h.subs {
		select {
		case ch <- ev:
		default:
		}
	}
	return ev
}

// Subscribe returns the events after lastID plus a channel for new ones. The
// replay and the subscription are taken together, so nothing falls in a gap.
func (h *Hub) Subscribe(lastID int64) ([]Event, <-chan Event, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch := make(chan Event, 64)
	if h.closed {
		close(ch)
		return nil, ch, func() {}
	}
	var replay []Event
	for _, ev := range h.ring {
		if ev.ID > lastID {
			replay = append(replay, ev)
		}
	}
	h.subs[ch] = struct{}{}
	cancel := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if _, ok := h.subs[ch]; ok {
			delete(h.subs, ch)
			close(ch)
		}
	}
	return replay, ch, cancel
}

// Recent returns up to limit of the newest events of a type ("" = all types).
func (h *Hub) Recent(typ string, limit int) []Event {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []Event
	for _, ev := range h.ring {
		if typ == "" || ev.Type == typ {
			out = append(out, ev)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}

func (h *Hub) LastID() int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.nextID
}

// Close ends every subscription so open SSE handlers return.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.closed = true
	for ch := range h.subs {
		close(ch)
	}
	h.subs = map[chan Event]struct{}{}
}
