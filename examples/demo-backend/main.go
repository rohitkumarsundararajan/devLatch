// Command demo-backend is a tiny JSON API used to develop and test DevLatch.
//
// It serves a few realistic endpoints and lets you "break" them on demand
// (rename a field, change a type, ...) so every DevLatch phase has a
// reproducible source of drift. It is not part of the shipped product.
package main

import (
	"flag"
	"log"
	"net/http"
	"strings"
	"time"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8000", "listen address")
	mutate := flag.String("mutate", "", "comma-separated mutations to enable at startup")
	flag.Parse()

	s := newServer()
	for _, name := range strings.Split(*mutate, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if !validMutation(name) {
			log.Fatalf("unknown mutation %q (available: %s)", name, strings.Join(allMutations, ", "))
		}
		s.set(name, true)
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           s.handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("demo backend listening on http://%s (mutations: %v)", *addr, s.activeList())
	log.Fatal(srv.ListenAndServe())
}
