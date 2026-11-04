package main

import (
	"strings"
	"testing"
)

func TestParseSpec(t *testing.T) {
	cases := []struct {
		in       string
		name     string
		env      []string
		argv     []string
		patterns []string
		wantErr  bool
	}{
		{
			in:       "NORMAL: go test -race -coverprofile=c.txt ./...",
			name:     "NORMAL",
			env:      []string{},
			argv:     []string{"go", "test", "-json", "-race", "-coverprofile=c.txt", "./..."},
			patterns: []string{"./..."},
		},
		{
			in:       "32BIT:  GOARCH=386 CGO_ENABLED=0 go test ./a/ ./b/...",
			name:     "32BIT",
			env:      []string{"GOARCH=386", "CGO_ENABLED=0"},
			argv:     []string{"go", "test", "-json", "./a/", "./b/..."},
			patterns: []string{"./a/", "./b/..."},
		},
		{in: "go test ./...", wantErr: true},
		{in: "NAME: make test", wantErr: true},
		{in: ": go test ./...", wantErr: true},
	}

	for _, tc := range cases {
		spec, err := parseSpec(tc.in)
		if (err != nil) != tc.wantErr {
			t.Errorf("%q: err = %v, wantErr %v", tc.in, err, tc.wantErr)

			continue
		}

		if err != nil {
			continue
		}

		if spec.name != tc.name || strings.Join(spec.env, " ") != strings.Join(tc.env, " ") ||
			strings.Join(spec.argv, " ") != strings.Join(tc.argv, " ") ||
			strings.Join(spec.patterns, " ") != strings.Join(tc.patterns, " ") {
			t.Errorf("%q: got %+v", tc.in, spec)
		}
	}
}

func TestHelpers(t *testing.T) {
	if got := escapeData("50% done\nnext, line: x"); got != "50%25 done%0Anext, line: x" {
		t.Errorf("escapeData = %q", got)
	}

	if got := escapeProperty("a:b,c%"); got != "a%3Ab%2Cc%25" {
		t.Errorf("escapeProperty = %q", got)
	}

	for sec, want := range map[float64]string{0: "0.0s", 5.44: "5.4s", 59.99: "60.0s", 60: "1m00s", 466.4: "7m46s"} {
		if got := fmtSeconds(sec); got != want {
			t.Errorf("fmtSeconds(%v) = %q, want %q", sec, got, want)
		}
	}

	if got := progressBar(1, 4); got != "█████░░░░░░░░░░░░░░░" {
		t.Errorf("progressBar = %q", got)
	}

	if got := progressBar(1, 0); got != "" {
		t.Errorf("progressBar without total = %q", got)
	}

	if topLevel("A/b/c") != "A" || topLevel("A") != "A" {
		t.Error("topLevel")
	}

	if !isRunMarker("=== RUN   TestX") || !isRunMarker("    === PAUSE TestX") || isRunMarker("--- PASS: TestX") {
		t.Error("isRunMarker")
	}
}
