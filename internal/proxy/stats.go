package proxy

import (
	"fmt"
	"sync/atomic"
)

type counters struct {
	seen             atomic.Int64 // JSON responses considered
	queued           atomic.Int64 // handed to the analysis queue
	analyzed         atomic.Int64 // merged into the store
	dropped          atomic.Int64 // queue full (or proxy closing): the app was never slowed
	skippedSampling  atomic.Int64
	skippedTooLarge  atomic.Int64
	skippedEncoding  atomic.Int64
	skippedStreaming atomic.Int64
	skippedNonJSON   atomic.Int64
	decodeErrors     atomic.Int64
	aborted          atomic.Int64 // client went away before the body finished
}

// Stats is a snapshot of the proxy's counters.
type Stats struct {
	JSONSeen         int64
	Queued           int64
	Analyzed         int64
	DroppedQueueFull int64
	SkippedSampling  int64
	SkippedTooLarge  int64
	SkippedEncoding  int64
	SkippedStreaming int64
	SkippedNonJSON   int64
	DecodeErrors     int64
	Aborted          int64
}

// Stats returns the current counters.
func (p *Proxy) Stats() Stats {
	return Stats{
		JSONSeen:         p.c.seen.Load(),
		Queued:           p.c.queued.Load(),
		Analyzed:         p.c.analyzed.Load(),
		DroppedQueueFull: p.c.dropped.Load(),
		SkippedSampling:  p.c.skippedSampling.Load(),
		SkippedTooLarge:  p.c.skippedTooLarge.Load(),
		SkippedEncoding:  p.c.skippedEncoding.Load(),
		SkippedStreaming: p.c.skippedStreaming.Load(),
		SkippedNonJSON:   p.c.skippedNonJSON.Load(),
		DecodeErrors:     p.c.decodeErrors.Load(),
		Aborted:          p.c.aborted.Load(),
	}
}

// String formats the counters on two lines for the terminal.
func (s Stats) String() string {
	return fmt.Sprintf(
		"JSON responses: %d seen, %d analyzed, %d dropped (queue full)\n"+
			"skipped: %d sampled out, %d too large, %d unsupported encoding, %d streaming, %d non-JSON | decode errors: %d | aborted: %d",
		s.JSONSeen, s.Analyzed, s.DroppedQueueFull,
		s.SkippedSampling, s.SkippedTooLarge, s.SkippedEncoding, s.SkippedStreaming, s.SkippedNonJSON,
		s.DecodeErrors, s.Aborted)
}
