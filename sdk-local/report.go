// Verify pinned selected-case coverage and split real integration from contracts.
package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"strings"
)

type Case struct {
	Class   string  `xml:"classname,attr"`
	Name    string  `xml:"name,attr"`
	Failure *string `xml:"failure"`
	Error   *string `xml:"error"`
	Skip    *string `xml:"skipped"`
}
type Suite struct {
	Cases  []Case  `xml:"testcase"`
	Suites []Suite `xml:"testsuite"`
}

func main() {
	if len(os.Args) != 4 {
		panic("usage: report manifest.json results.xml folder")
	}
	b, e := os.ReadFile(os.Args[1])
	if e != nil {
		panic(e)
	}
	var all map[string]bool
	if e = json.Unmarshal(b, &all); e != nil {
		panic(e)
	}
	expected := map[string]bool{}
	for k, v := range all {
		if strings.Contains(k, "."+os.Args[3]+".") {
			expected[k] = v
		}
	}
	b, e = os.ReadFile(os.Args[2])
	if e != nil {
		panic(e)
	}
	var tree Suite
	if e = xml.Unmarshal(b, &tree); e != nil {
		panic(e)
	}
	counts := map[string]int{}
	bad := false
	var visit func(Suite)
	visit = func(s Suite) {
		for _, c := range s.Cases {
			key := c.Class + "::" + c.Name
			integration, ok := expected[key]
			if !ok {
				fmt.Printf("Unexpected testcase: %s\n", key)
				bad = true
				continue
			}
			delete(expected, key)
			kind := "contract"
			if integration {
				kind = "integration"
			}
			state := "pass"
			if c.Skip != nil {
				state = "skip"
				bad = true
			}
			if c.Failure != nil {
				state = "fail"
				bad = true
			}
			if c.Error != nil {
				state = "error"
				bad = true
			}
			counts[kind+"_"+state]++
			if state != "pass" {
				fmt.Printf("%s %s %s\n", kind, state, key)
			}
		}
		for _, child := range s.Suites {
			visit(child)
		}
	}
	visit(tree)
	for k, v := range expected {
		fmt.Printf("NOT_EXECUTED integration=%v %s\n", v, k)
		bad = true
	}
	fmt.Printf("Counts: %v; not_executed=%d\n", counts, len(expected))
	if bad || len(counts) == 0 {
		os.Exit(1)
	}
}
