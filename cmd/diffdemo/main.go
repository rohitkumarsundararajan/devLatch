// Throwaway demo for Phase 2: shows diff output on hand-built schemas.
// It is removed once Phase 3 gives us the real `devlatch check` command.
package main

import (
	"flag"
	"os"

	"github.com/rohitkumarsundararajan/devlatch/internal/diff"
	"github.com/rohitkumarsundararajan/devlatch/internal/infer"
	"github.com/rohitkumarsundararajan/devlatch/internal/report"
)

func leaf(n int, k string) *infer.Node {
	return &infer.Node{Kinds: map[infer.Kind]int{infer.Kind(k): n}, Samples: n}
}

func obj(n int, p map[string]*infer.Prop) *infer.Node {
	return &infer.Node{Kinds: map[infer.Kind]int{"OBJECT": n}, Properties: p, Samples: n}
}

func main() {
	asJSON := flag.Bool("json", false, "print JSON instead of text")
	flag.Parse()

	base := obj(30, map[string]*infer.Prop{
		"id":      {Node: leaf(30, "INTEGER"), SeenIn: 30},
		"role_id": {Node: leaf(30, "INTEGER"), SeenIn: 30},
		"price":   {Node: leaf(30, "NUMBER"), SeenIn: 30},
		"name":    {Node: leaf(30, "STRING"), SeenIn: 30},
	})
	live := obj(30, map[string]*infer.Prop{
		"id":    {Node: leaf(30, "INTEGER"), SeenIn: 30},
		"price": {Node: leaf(30, "STRING"), SeenIn: 30},
		"name":  {Node: &infer.Node{Kinds: map[infer.Kind]int{"STRING": 20, "NULL": 10}, Samples: 30}, SeenIn: 30},
		"email": {Node: leaf(30, "STRING"), SeenIn: 30},
	})
	ep := "GET /api/users/{id} 200"
	vs := diff.Diff(map[string]*infer.Node{ep: base}, map[string]*infer.Node{ep: live}, diff.Options{})

	if *asJSON {
		_ = report.JSON(os.Stdout, vs)
	} else {
		report.Text(os.Stdout, vs, true)
	}
	os.Exit(diff.ExitCode(vs, ""))
}
