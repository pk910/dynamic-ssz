package main

import (
	"encoding/json"
	"strings"
	"time"
)

// testEvent is one line of `go test -json` output. build-output and
// build-fail events (Go 1.24+) carry ImportPath instead of Package.
type testEvent struct {
	Time       time.Time
	Action     string
	Package    string
	ImportPath string
	Test       string
	Elapsed    float64
	Output     string
}

// parseEvent decodes one output line. ok is false for lines that are not
// JSON events: compiler errors on Go < 1.24, `go: downloading ...`, and
// anything else the go command writes to stderr.
func parseEvent(line string) (ev testEvent, ok bool) {
	if line == "" || line[0] != '{' {
		return testEvent{}, false
	}

	if err := json.Unmarshal([]byte(line), &ev); err != nil || ev.Action == "" {
		return testEvent{}, false
	}

	return ev, true
}

// packageOf returns the package an event belongs to. Build events name the
// test binary as "pkg [pkg.test]"; the bracket suffix is dropped.
func (ev *testEvent) packageOf() string {
	if ev.Package != "" {
		return ev.Package
	}

	if i := strings.Index(ev.ImportPath, " ["); i >= 0 {
		return ev.ImportPath[:i]
	}

	return ev.ImportPath
}

// topLevel returns the top-level test name of a (sub)test.
func topLevel(test string) string {
	if i := strings.IndexByte(test, '/'); i >= 0 {
		return test[:i]
	}

	return test
}

// isRunMarker reports whether a verbose output line is a === RUN / PAUSE /
// CONT / NAME marker. Those carry no information beyond the --- PASS line
// that follows and are dropped from the rendered log.
func isRunMarker(text string) bool {
	t := strings.TrimLeft(text, " ")

	return strings.HasPrefix(t, "=== ")
}
