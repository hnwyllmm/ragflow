// Command verify-api rejects missing, failed or skipped selected API tests.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func main() {
	cases, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	wanted := map[string]bool{}
	ran := map[string]bool{}
	passed := map[string]bool{}
	for _, name := range strings.Fields(string(cases)) {
		wanted[name] = true
	}
	file, err := os.Open(os.Args[2])
	if err != nil {
		panic(err)
	}
	defer file.Close()
	s := bufio.NewScanner(file)
	s.Buffer(make([]byte, 4096), 8<<20)
	bad := false
	for s.Scan() {
		var e struct{ Action, Test string }
		if err := json.Unmarshal(s.Bytes(), &e); err != nil {
			panic(err)
		}
		root := strings.SplitN(e.Test, "/", 2)[0]
		if !wanted[root] {
			continue
		}
		if e.Action == "skip" || e.Action == "fail" {
			fmt.Printf("Rejected %s: %s\n", e.Action, e.Test)
			bad = true
		}
		if e.Test == root && e.Action == "run" {
			ran[root] = true
		}
		if e.Test == root && e.Action == "pass" {
			passed[root] = true
		}
	}
	if err := s.Err(); err != nil {
		panic(err)
	}
	for name := range wanted {
		if !ran[name] || !passed[name] {
			fmt.Printf("Missing RUN/PASS: %s\n", name)
			bad = true
		}
	}
	if bad {
		os.Exit(1)
	}
	fmt.Printf("All %d selected tests ran and passed without skips\n", len(wanted))
}
