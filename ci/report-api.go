// Summarize broad upstream tests without treating skips as passes.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func main() {
	f, err := os.Open(os.Args[1])
	if err != nil {
		panic(err)
	}
	defer f.Close()
	type result struct{ Action, Test, Output string }
	states := map[string]string{}
	reasons := map[string]string{}
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 4096), 8<<20)
	for s.Scan() {
		var e result
		if json.Unmarshal(s.Bytes(), &e) != nil {
			continue
		}
		if e.Test == "" {
			continue
		}
		if e.Action == "run" {
			states[e.Test] = "unfinished"
		}
		if e.Action == "pass" || e.Action == "fail" || e.Action == "skip" {
			states[e.Test] = e.Action
		}
		if e.Action == "output" {
			reasons[e.Test] += e.Output
		}
	}
	if err := s.Err(); err != nil {
		panic(err)
	}
	totals := map[string]int{}
	roots := map[string]int{}
	for name, state := range states {
		totals[state]++
		if !strings.Contains(name, "/") {
			roots[state]++
		}
	}
	fmt.Printf("## Broad upstream Go API results\n\nTop-level tests: %v\nAll test/subtest events (not unique scenarios): %v\n", roots, totals)
	for _, status := range []string{"fail", "skip", "unfinished"} {
		fmt.Printf("\n### %s\n", status)
		for name, state := range states {
			if state == status {
				fmt.Printf("\n%s\n```text\n%s```\n", name, reasons[name])
			}
		}
	}
	if len(states) == 0 {
		fmt.Println("No tests executed: bootstrap/build/collection failure, not a pass")
		os.Exit(1)
	}
	if totals["fail"]+totals["unfinished"] > 0 {
		os.Exit(1)
	}
}
