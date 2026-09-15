package main

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden files")

type fixtureCase struct {
	name     string
	fixture  string
	github   bool
	perTest  bool
	total    int
	exitCode int
	wantOK   bool
}

// replay feeds a recorded go test -json stream through a run and returns
// the rendered log and the run for further assertions.
func replay(t *testing.T, tc *fixtureCase) (string, *run) {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("testdata", tc.fixture+".json"))
	if err != nil {
		t.Fatal(err)
	}

	cfg := &config{module: "example.com/fx", github: tc.github, perTest: tc.perTest, workDir: "sub"}

	var buf bytes.Buffer

	out := newPrinter(&buf, tc.github, false, []string{"NORMAL", "X"})

	r := newRun(&runSpec{name: "NORMAL"}, cfg, out)
	r.total = tc.total

	r.consume(strings.NewReader(string(data)))
	r.finish(tc.exitCode, nil)

	out.block(func(b *strings.Builder) {
		b.WriteString(r.resultLine())
		b.WriteByte('\n')
	})

	return buf.String(), r
}

func checkGolden(t *testing.T, name, got string) {
	t.Helper()

	path := filepath.Join("testdata", name+".golden")

	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil { //nolint:gosec // test fixture
			t.Fatal(err)
		}
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to create)", err)
	}

	if string(want) != got {
		t.Errorf("%s differs from golden file:\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

func TestRender(t *testing.T) {
	cases := []fixtureCase{
		{name: "pass-github", fixture: "pass", github: true, total: 2, exitCode: 0, wantOK: true},
		{name: "mixed-github", fixture: "mixed", github: true, total: 4, exitCode: 1},
		{name: "mixed-local", fixture: "mixed", github: false, total: 4, exitCode: 1},
		{name: "mixed-pertest", fixture: "mixed", github: true, perTest: true, total: 4, exitCode: 1},
		{name: "buildfail-github", fixture: "buildfail", github: true, total: 1, exitCode: 1},
		{name: "buildfail-go122-github", fixture: "buildfail-go122", github: true, total: 2, exitCode: 1},
	}

	for i := range cases {
		tc := &cases[i]

		t.Run(tc.name, func(t *testing.T) {
			got, r := replay(t, tc)

			checkGolden(t, tc.name, got)

			if r.ok() != tc.wantOK {
				t.Errorf("ok() = %v, want %v", r.ok(), tc.wantOK)
			}
		})
	}
}

func TestCounters(t *testing.T) {
	_, r := replay(t, &fixtureCase{fixture: "mixed", github: true, total: 4, exitCode: 1})

	if r.testedPkgs != 3 || r.noTestPkgs != 1 || r.failedPkgs != 2 {
		t.Errorf("packages: tested=%d notest=%d failed=%d", r.testedPkgs, r.noTestPkgs, r.failedPkgs)
	}

	if r.passed != 8 || r.skipped != 3 || r.failed != 3 {
		t.Errorf("tests: passed=%d skipped=%d failed=%d", r.passed, r.skipped, r.failed)
	}

	// The parent of a failed subtest is not listed; the panic is located
	// through its trace.
	if len(r.failures) != 2 {
		t.Fatalf("failures = %+v", r.failures)
	}

	if f := r.failures[0]; f.test != "TestSubs/case-1" || f.file != "alpha_test.go" || f.line != 18 {
		t.Errorf("failure[0] = %+v", f)
	}

	if f := r.failures[1]; f.test != "TestPanics" || f.file != "beta_test.go" || f.line != 9 || !strings.HasPrefix(f.msg, "panic: ") {
		t.Errorf("failure[1] = %+v", f)
	}

	if len(r.skips) != 3 || r.skips[0].reason != "needs a 64-bit platform" {
		t.Errorf("skips = %+v", r.skips)
	}
}

func TestStrayLines(t *testing.T) {
	_, r := replay(t, &fixtureCase{fixture: "buildfail-go122", github: true, total: 2, exitCode: 1})

	if r.stray != 3 {
		t.Errorf("stray = %d, want 3", r.stray)
	}

	if r.failedPkgs != 1 || r.testedPkgs != 2 {
		t.Errorf("packages: tested=%d failed=%d", r.testedPkgs, r.failedPkgs)
	}
}

func TestSummary(t *testing.T) {
	_, r1 := replay(t, &fixtureCase{fixture: "mixed", github: true, total: 4, exitCode: 1})
	_, r2 := replay(t, &fixtureCase{fixture: "buildfail-go122", github: true, total: 2, exitCode: 1})

	r2.spec.name = "X"

	checkGolden(t, "summary", renderSummary(&config{title: "Scratch tests"}, []*run{r1, r2}))
}

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
