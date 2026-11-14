// Package proxy is DevLatch's reverse proxy. It forwards every request to the
// target untouched and keeps a bounded copy of JSON responses for analysis.
//
// The rule that shapes everything here: analysis must never slow the app.
// Bytes are streamed to the client as they arrive; the copy is handed to a
// bounded queue without ever blocking, and is dropped (and counted) if the
// queue is full.
package proxy

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/OWNER/devlatch/internal/infer"
	"github.com/OWNER/devlatch/internal/normalize"
)

// Options configures a Proxy. Zero values take the documented defaults.
type Options struct {
	Target         *url.URL     // backend to forward to (scheme and host required)
	Store          *infer.Store // where structure is accumulated; a new one if nil
	QueueSize      int          // analysis queue capacity (default 64)
	Workers        int          // analysis goroutines (default 2)
	MaxBodyBytes   int          // largest body analyzed, compressed or not (default 1 MiB)
	SampleEvery    int          // analyze every Nth JSON response (default 1 = all)
	IgnorePrefixes []string     // request path prefixes that are forwarded but never analyzed
	Logger         *log.Logger  // default log.Default()

	OnSample func(method, path string, status int, body []byte) // called for every analyzed response

	noWorkers bool // tests: leave the queue undrained to prove the proxy never blocks
}

type sample struct {
	method   string
	path     string
	status   int
	encoding string
	body     []byte
}

// Proxy is an http.Handler. Create it with New and call Close when done.
type Proxy struct {
	opts  Options
	rp    *httputil.ReverseProxy
	store *infer.Store
	queue chan sample
	done  chan struct{}
	wg    sync.WaitGroup
	once  sync.Once
	seq   atomic.Uint64
	c     counters
}

type inboundPathKey struct{}

// New validates opts and starts the analysis workers.
func New(opts Options) (*Proxy, error) {
	if opts.Target == nil || opts.Target.Scheme == "" || opts.Target.Host == "" {
		return nil, errors.New("proxy: target must be a URL with scheme and host, e.g. http://localhost:8000")
	}
	if opts.QueueSize <= 0 {
		opts.QueueSize = 64
	}
	if opts.Workers <= 0 {
		opts.Workers = 2
	}
	if opts.MaxBodyBytes <= 0 {
		opts.MaxBodyBytes = 1 << 20
	}
	if opts.SampleEvery <= 0 {
		opts.SampleEvery = 1
	}
	if opts.Logger == nil {
		opts.Logger = log.Default()
	}
	p := &Proxy{
		opts:  opts,
		store: opts.Store,
		queue: make(chan sample, opts.QueueSize),
		done:  make(chan struct{}),
	}
	if p.store == nil {
		p.store = infer.NewStore(infer.DefaultConfig())
	}
	p.rp = &httputil.ReverseProxy{
		Rewrite:        p.rewrite,
		ModifyResponse: p.modifyResponse,
		ErrorHandler:   p.errorHandler,
	}
	if !opts.noWorkers {
		for i := 0; i < opts.Workers; i++ {
			p.wg.Add(1)
			go p.worker()
		}
	}
	return p, nil
}

// Store returns the store the proxy feeds.
func (p *Proxy) Store() *infer.Store { return p.store }

// ServeHTTP forwards the request to the target.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// The outbound request's path is rewritten (the target may have a base
	// path), so remember the path the client actually used for endpoint keys.
	ctx := context.WithValue(r.Context(), inboundPathKey{}, r.URL.Path)
	p.rp.ServeHTTP(w, r.WithContext(ctx))
}

// Close stops the workers and analyzes whatever is still queued. Stop the
// HTTP server first so no new responses arrive.
func (p *Proxy) Close() {
	p.once.Do(func() {
		close(p.done)
		p.wg.Wait()
		for {
			select {
			case s := <-p.queue:
				p.process(s)
			default:
				return
			}
		}
	})
}

func (p *Proxy) rewrite(pr *httputil.ProxyRequest) {
	pr.SetURL(p.opts.Target)
	// Go's http package cannot decompress br/zstd without extra dependencies,
	// so never invite them; gzip and deflate are analyzed.
	if ae := pr.In.Header.Get("Accept-Encoding"); ae != "" {
		if kept := keepSupportedEncodings(ae); kept != "" {
			pr.Out.Header.Set("Accept-Encoding", kept)
		} else {
			pr.Out.Header.Del("Accept-Encoding")
		}
	}
}

func keepSupportedEncodings(header string) string {
	var kept []string
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		name := strings.ToLower(strings.TrimSpace(strings.SplitN(part, ";", 2)[0]))
		switch name {
		case "gzip", "x-gzip", "deflate", "identity":
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, ", ")
}

func (p *Proxy) errorHandler(w http.ResponseWriter, r *http.Request, err error) {
	p.opts.Logger.Printf("devlatch: upstream error for %s %s: %v", r.Method, r.URL.Path, err)
	w.WriteHeader(http.StatusBadGateway)
}

func (p *Proxy) modifyResponse(resp *http.Response) error {
	req := resp.Request
	path, _ := req.Context().Value(inboundPathKey{}).(string)
	if path == "" {
		path = req.URL.Path
	}
	for _, prefix := range p.opts.IgnorePrefixes {
		if strings.HasPrefix(path, prefix) {
			return nil
		}
	}
	status := resp.StatusCode
	if status < 200 || status == http.StatusNoContent || status == http.StatusNotModified ||
		req.Method == http.MethodHead || resp.Body == nil || resp.Body == http.NoBody {
		return nil
	}

	mt, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	switch {
	case mt == "text/event-stream":
		p.c.skippedStreaming.Add(1)
		return nil
	case mt != "application/json" && !strings.HasSuffix(mt, "+json"):
		p.c.skippedNonJSON.Add(1)
		return nil
	}
	p.c.seen.Add(1)

	enc := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding")))
	switch enc {
	case "", "identity", "gzip", "x-gzip", "deflate":
	default:
		p.c.skippedEncoding.Add(1)
		return nil
	}
	if (p.seq.Add(1)-1)%uint64(p.opts.SampleEvery) != 0 {
		p.c.skippedSampling.Add(1)
		return nil
	}
	if resp.ContentLength > int64(p.opts.MaxBodyBytes) {
		p.c.skippedTooLarge.Add(1)
		return nil
	}

	s := sample{method: req.Method, path: path, status: status, encoding: enc}
	resp.Body = &teeBody{
		rc:  resp.Body,
		max: p.opts.MaxBodyBytes,
		onDone: func(body []byte, complete, truncated bool) {
			switch {
			case truncated:
				p.c.skippedTooLarge.Add(1)
			case !complete:
				p.c.aborted.Add(1)
			default:
				s.body = body
				p.enqueue(s)
			}
		},
	}
	return nil
}

// enqueue never blocks: a full queue (or a closing proxy) drops the sample.
func (p *Proxy) enqueue(s sample) {
	select {
	case <-p.done:
		p.c.dropped.Add(1)
		return
	default:
	}
	select {
	case p.queue <- s:
		p.c.queued.Add(1)
	default:
		p.c.dropped.Add(1)
	}
}

func (p *Proxy) worker() {
	defer p.wg.Done()
	for {
		select {
		case s := <-p.queue:
			p.process(s)
		case <-p.done:
			return
		}
	}
}

func (p *Proxy) process(s sample) {
	defer func() {
		if r := recover(); r != nil { // a strange payload must never take DevLatch down
			p.c.decodeErrors.Add(1)
			p.opts.Logger.Printf("devlatch: recovered while analyzing %s %s: %v", s.method, s.path, r)
		}
	}()
	body, err := p.decompress(s)
	switch {
	case errors.Is(err, errTooLarge):
		p.c.skippedTooLarge.Add(1)
		return
	case err != nil:
		p.c.decodeErrors.Add(1)
		return
	}
	key := normalize.EndpointKey(s.method, s.path, s.status)
	if err := p.store.Observe(key, body); err != nil {
		p.c.decodeErrors.Add(1)
		return
	}
	p.c.analyzed.Add(1)
	if p.opts.OnSample != nil {
		p.opts.OnSample(s.method, s.path, s.status, body)
	}
}

var errTooLarge = errors.New("decompressed body too large")

func (p *Proxy) decompress(s sample) ([]byte, error) {
	var r io.Reader
	switch s.encoding {
	case "", "identity":
		return s.body, nil
	case "gzip", "x-gzip":
		zr, err := gzip.NewReader(bytes.NewReader(s.body))
		if err != nil {
			return nil, err
		}
		defer func() { _ = zr.Close() }()
		r = zr
	case "deflate": // "deflate" is zlib-wrapped per the RFC, but some servers send raw deflate
		if zr, err := zlib.NewReader(bytes.NewReader(s.body)); err == nil {
			defer func() { _ = zr.Close() }()
			r = zr
		} else {
			r = flate.NewReader(bytes.NewReader(s.body))
		}
	default:
		return nil, fmt.Errorf("unsupported encoding %q", s.encoding)
	}
	// Cap the decompressed size too, so a tiny compressed body cannot balloon.
	data, err := io.ReadAll(io.LimitReader(r, int64(p.opts.MaxBodyBytes)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > p.opts.MaxBodyBytes {
		return nil, errTooLarge
	}
	return data, nil
}
