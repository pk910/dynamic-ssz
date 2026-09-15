package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// locationRE matches the "file.go:12: message" lines t.Log, t.Error and
// t.Skip produce inside a test's output.
var locationRE = regexp.MustCompile(`^\s*([\w./-]+\.go):(\d+): ?(.*)$`)

// traceRE matches the first test-file frame of a panic trace.
var traceRE = regexp.MustCompile(`^\s+(?:\S*/)?([\w.-]+_test\.go):(\d+) `)

const (
	defaultNameWidth = 32
	maxNameWidth     = 60

	actionPass = "pass"
	actionFail = "fail"
	actionSkip = "skip"

	statusRunning = 'r'
	statusPassed  = 'p'
	statusFailed  = 'f'
	statusSkipped = 's'
)

type outLine struct {
	test string
	text string
}

type topTest struct {
	started time.Time
	passed  int
	skipped int
	failed  int
}

type pkgState struct {
	path    string
	started time.Time
	lines   []outLine
	status  map[string]byte
	tops    map[string]*topTest
	passed  int
	skipped int
	failed  int
	built   bool
	done    bool
}

type failure struct {
	pkg  string
	test string
	file string
	line int
	msg  string
}

type skipped struct {
	pkg    string
	test   string
	reason string
}

type pkgTime struct {
	pkg     string
	elapsed float64
}

// detail is the verbose output of one package (or top-level test), kept
// for the collapsed groups printed once every run has finished: groups
// streamed live render expanded in the GitHub log viewer.
type detail struct {
	title string
	lines []string
}

// run executes one `go test` variant and turns its event stream into
// progress lines, collapsed details and summary counters.
type run struct {
	spec *runSpec
	cfg  *config
	out  *printer

	mu         sync.Mutex
	pkgs       map[string]*pkgState
	total      int
	donePkgs   int
	testedPkgs int
	noTestPkgs int
	failedPkgs int
	passed     int
	skipped    int
	failed     int
	failures   []failure
	skips      []skipped
	times      []pkgTime
	details    []detail
	stray      int
	nameWidth  int
	firstEvent time.Time
	lastEvent  time.Time
	lastPrint  time.Time
	exitCode   int
	exitErr    string

	log *bufio.Writer
}

func newRun(spec *runSpec, cfg *config, out *printer) *run {
	return &run{
		spec:      spec,
		cfg:       cfg,
		out:       out,
		pkgs:      make(map[string]*pkgState, 32),
		nameWidth: defaultNameWidth,
	}
}

func (r *run) prefix() string {
	return r.out.prefix(r.spec.name)
}

// displayName strips the module path from an import path.
func (r *run) displayName(pkg string) string {
	if pkg == r.cfg.module {
		return path.Base(pkg)
	}

	return strings.TrimPrefix(pkg, r.cfg.module+"/")
}

// pkgDir returns the repo-relative directory of a package, for annotations.
func (r *run) pkgDir(pkg string) string {
	rel := strings.TrimPrefix(pkg, r.cfg.module)
	rel = strings.TrimPrefix(rel, "/")

	return path.Join(r.cfg.workDir, rel)
}

// prepare resolves the package list so the global progress bar knows its
// total before any run starts.
func (r *run) prepare(ctx context.Context) {
	r.countPackages(ctx)
	r.out.addTotal(r.total)
}

// execute runs the variant to completion.
func (r *run) execute(ctx context.Context) error {
	if r.cfg.logDir != "" {
		f, err := os.Create(path.Join(r.cfg.logDir, r.spec.name+".log"))
		if err != nil {
			return fmt.Errorf("create log: %w", err)
		}

		defer func() { _ = f.Close() }()

		r.log = bufio.NewWriterSize(f, 64*1024)

		defer func() { _ = r.log.Flush() }()
	}

	cmd := exec.CommandContext(ctx, r.spec.argv[0], r.spec.argv[1:]...) //nolint:gosec // the command line is the operator's own -cmd flag
	cmd.Env = append(os.Environ(), r.spec.env...)

	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		_ = pw.Close()

		return fmt.Errorf("start %s: %w", r.spec.name, err)
	}

	var wg sync.WaitGroup

	wg.Add(1)

	go func() {
		defer wg.Done()

		r.consume(pr)
	}()

	stopBeat := r.startHeartbeat(ctx)

	waitErr := cmd.Wait()

	_ = pw.Close()

	wg.Wait()
	stopBeat()

	code := 0

	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		code = exitErr.ExitCode()
	} else if waitErr != nil {
		code = 1
	}

	r.finish(code, waitErr)

	return nil
}

// countPackages resolves the run's package patterns with go list so the
// progress counter has a denominator and package names align. Failures
// leave the total at zero.
func (r *run) countPackages(ctx context.Context) {
	if len(r.spec.patterns) == 0 {
		return
	}

	args := append([]string{"list", "-e", "-f", "{{.ImportPath}}"}, r.spec.patterns...)

	cmd := exec.CommandContext(ctx, goCmd, args...) //nolint:gosec // the patterns come from the operator's own -cmd flag
	cmd.Env = append(os.Environ(), r.spec.env...)

	outBytes, err := cmd.Output()
	if err != nil {
		return
	}

	pkgs := strings.Fields(string(outBytes))
	r.total = len(pkgs)

	if r.cfg.perTest {
		return
	}

	width := 0
	for _, p := range pkgs {
		if n := len(r.displayName(p)); n > width {
			width = n
		}
	}

	if width > maxNameWidth {
		width = maxNameWidth
	}

	if width > 0 {
		r.nameWidth = width
	}
}

// consume reads the merged stdout/stderr stream line by line until EOF.
func (r *run) consume(in io.Reader) {
	rd := bufio.NewReaderSize(in, 256*1024)

	for {
		line, err := rd.ReadString('\n')
		if line != "" {
			r.handleLine(strings.TrimSuffix(line, "\n"))
		}

		if err != nil {
			return
		}
	}
}

func (r *run) handleLine(line string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	ev, ok := parseEvent(line)
	if !ok {
		r.stray++
		r.logWrite(line + "\n")
		r.out.block(func(b *strings.Builder) {
			b.WriteString(r.prefix())
			b.WriteString(line)
			b.WriteByte('\n')
		})

		return
	}

	if r.firstEvent.IsZero() {
		r.firstEvent = ev.Time
	}

	r.lastEvent = ev.Time

	p := r.pkg(ev.packageOf())

	switch ev.Action {
	case "start":
		p.started = time.Now()
	case "run":
		p.status[ev.Test] = statusRunning

		if ev.Test == topLevel(ev.Test) {
			p.tops[ev.Test] = &topTest{started: time.Now()}
		}
	case "output", "build-output":
		r.logWrite(ev.Output)

		for _, text := range strings.Split(strings.TrimSuffix(ev.Output, "\n"), "\n") {
			p.lines = append(p.lines, outLine{test: ev.Test, text: text})

			if strings.HasSuffix(text, "[build failed]") {
				p.built = false
			}
		}
	case "build-fail":
		p.built = false
	case actionPass, actionFail, actionSkip:
		if ev.Test != "" {
			r.testDone(p, &ev)
		} else {
			r.pkgDone(p, &ev)
		}
	}
}

func (r *run) pkg(name string) *pkgState {
	p, ok := r.pkgs[name]
	if !ok {
		p = &pkgState{
			path:   name,
			status: make(map[string]byte, 64),
			tops:   make(map[string]*topTest, 16),
			built:  true,
		}
		r.pkgs[name] = p
	}

	return p
}

func (r *run) logWrite(s string) {
	if r.log != nil {
		_, _ = r.log.WriteString(s)
	}
}

func (r *run) testDone(p *pkgState, ev *testEvent) {
	top := topLevel(ev.Test)

	ts, ok := p.tops[top]
	if !ok {
		ts = &topTest{started: time.Now()}
		p.tops[top] = ts
	}

	isSub := ev.Test != top

	switch ev.Action {
	case actionPass:
		p.status[ev.Test] = statusPassed
		p.passed++
		r.passed++

		if isSub {
			ts.passed++
		}
	case actionSkip:
		p.status[ev.Test] = statusSkipped
		p.skipped++
		r.skipped++

		if isSub {
			ts.skipped++
		}

		_, _, reason := r.location(p, ev.Test)
		r.skips = append(r.skips, skipped{pkg: r.displayName(p.path), test: ev.Test, reason: reason})
	case actionFail:
		p.status[ev.Test] = statusFailed
		p.failed++
		r.failed++

		if isSub {
			ts.failed++
		}

		file, line, msg := r.location(p, ev.Test)
		if file == "" {
			file, line, msg = r.panicLocation(p, ev.Test)
		}

		// A parent that fails only because a subtest failed adds nothing.
		if file != "" || msg != "" || !r.hasFailedSubtest(ev.Test) {
			r.failures = append(r.failures, failure{pkg: r.displayName(p.path), test: ev.Test, file: file, line: line, msg: msg})
		}
	}

	if !isSub && r.cfg.perTest {
		r.renderTop(p, ev, ts)
		delete(p.tops, top)
	}
}

// location finds the first "file.go:N: message" line in a test's own output.
func (r *run) location(p *pkgState, test string) (file string, line int, msg string) {
	for _, l := range p.lines {
		if l.test != test {
			continue
		}

		m := locationRE.FindStringSubmatch(l.text)
		if m == nil {
			continue
		}

		n, _ := strconv.Atoi(m[2])

		return m[1], n, m[3]
	}

	return "", 0, ""
}

// panicLocation derives file, line and message of a test that died in a
// panic from the trace attributed to it.
func (r *run) panicLocation(p *pkgState, test string) (file string, line int, msg string) {
	for _, l := range p.lines {
		if l.test != test {
			continue
		}

		if msg == "" && strings.HasPrefix(l.text, "panic: ") {
			msg = l.text

			continue
		}

		if m := traceRE.FindStringSubmatch(l.text); m != nil && msg != "" {
			n, _ := strconv.Atoi(m[2])

			return m[1], n, msg
		}
	}

	return "", 0, msg
}

func (r *run) hasFailedSubtest(test string) bool {
	for i := len(r.failures) - 1; i >= 0; i-- {
		if strings.HasPrefix(r.failures[i].test, test+"/") {
			return true
		}
	}

	return false
}

func (r *run) pkgDone(p *pkgState, ev *testEvent) {
	if p.done {
		return
	}

	p.done = true
	r.donePkgs++
	r.out.advance()

	if ev.Action == actionSkip {
		// [no test files]
		r.noTestPkgs++
		r.releaseLines(p, "")

		return
	}

	r.testedPkgs++
	r.times = append(r.times, pkgTime{pkg: r.displayName(p.path), elapsed: ev.Elapsed})

	if ev.Action == actionFail {
		r.failedPkgs++
	}

	r.lastPrint = time.Now()

	r.out.block(func(b *strings.Builder) {
		r.writePkgBlock(b, p, ev)
	})

	r.releaseLines(p, "")
}

// writePkgBlock renders the package line, keeps its verbose output for the
// deferred collapsed group, and prints any failure excerpt in the open.
func (r *run) writePkgBlock(b *strings.Builder, p *pkgState, ev *testEvent) {
	summary := counts(p.passed, p.skipped, p.failed)
	if !p.built {
		summary = "build failed"
	}

	// The live line leads with the global progress; the detail group later
	// carries only the per-run part.
	line := fmt.Sprintf("%s%s %-*s %7s %s", r.prefix(), r.mark(ev.Action),
		r.nameWidth, r.displayName(p.path), fmtSeconds(ev.Elapsed), summary)
	line = strings.TrimRight(line, " ")

	b.WriteString(r.colorize(r.out.progress() + " " + line))
	b.WriteByte('\n')

	if r.out.github {
		r.keepDetail(line, p, func(l outLine) bool {
			if r.cfg.perTest {
				return l.test == ""
			}

			return true
		})
	}

	// Per test, failures were shown as they happened; the package block only
	// adds something when the package failed without a failing test.
	if ev.Action == actionFail && (!r.cfg.perTest || p.failed == 0) {
		r.writeFailure(b, p, "", !r.cfg.perTest)
	}
}

func (r *run) mark(action string) string {
	switch action {
	case actionFail:
		return "✗"
	case actionSkip:
		return "-"
	default:
		return "✓"
	}
}

// colorize paints the marks and counts of a live package or test line.
func (r *run) colorize(line string) string {
	if !r.out.color {
		return line
	}

	rep := strings.NewReplacer(
		" ✓ ", " "+ansiGreen+"✓"+ansiReset+" ",
		" ✗ ", " "+ansiRed+"✗"+ansiReset+" ",
		" FAILED", " "+ansiRed+"FAILED"+ansiReset,
		" skipped", " "+ansiYellow+"skipped"+ansiReset,
		"build failed", ansiRed+"build failed"+ansiReset,
	)

	return rep.Replace(line)
}

// keepDetail stores the buffered output lines a filter selects, minus the
// === RUN markers, under the given group title.
func (r *run) keepDetail(title string, p *pkgState, keep func(outLine) bool) {
	lines := make([]string, 0, len(p.lines))

	for _, l := range p.lines {
		if !keep(l) || isRunMarker(l.text) {
			continue
		}

		lines = append(lines, l.text)
	}

	r.details = append(r.details, detail{title: title, lines: lines})
}

// renderTop renders one top-level test (per-test granularity) and drops its
// buffered lines.
func (r *run) renderTop(p *pkgState, ev *testEvent, ts *topTest) {
	r.lastPrint = time.Now()

	r.out.block(func(b *strings.Builder) {
		subs := ""
		if ts.passed+ts.skipped+ts.failed > 0 {
			subs = counts(ts.passed, ts.skipped, ts.failed) + " subtests"
		}

		title := fmt.Sprintf("%s%s %-*s %7s %s", r.prefix(), r.mark(ev.Action), r.nameWidth, ev.Test, fmtSeconds(ev.Elapsed), subs)
		title = strings.TrimRight(title, " ")

		b.WriteString(r.colorize(title))
		b.WriteByte('\n')

		if r.out.github {
			r.keepDetail(title, p, func(l outLine) bool { return topLevel(l.test) == ev.Test })
		}

		if ev.Action == actionFail {
			r.writeFailure(b, p, ev.Test, true)
		}
	})

	r.releaseLines(p, ev.Test)
}

// writeDetails prints the collapsed groups of verbose output, one per
// package or top-level test, in completion order.
func (r *run) writeDetails(b *strings.Builder) {
	if len(r.details) == 0 {
		return
	}

	fmt.Fprintf(b, "%s── verbose output, one collapsed group per %s ──\n", r.prefix(), r.detailUnit())

	for _, d := range r.details {
		r.out.groupStart(b, d.title)

		for _, l := range d.lines {
			b.WriteString(r.prefix())
			b.WriteString(l)
			b.WriteByte('\n')
		}

		r.out.groupEnd(b)
	}
}

func (r *run) detailUnit() string {
	if r.cfg.perTest {
		return "top-level test"
	}

	return "package"
}

// writeFailure prints, uncollapsed, the output of every test that failed or
// never finished (panics, timeouts), plus package-level output when the
// package failed without a failing test (build errors, TestMain). With top
// set only that top-level test is covered; annotate emits the ::error
// annotations of the covered failures.
func (r *run) writeFailure(b *strings.Builder, p *pkgState, top string, annotate bool) {
	for _, l := range p.lines {
		if top != "" && topLevel(l.test) != top {
			continue
		}

		if isRunMarker(l.text) {
			continue
		}

		if l.test != "" {
			st := p.status[l.test]
			if st != statusFailed && st != statusRunning {
				continue
			}
		} else if top != "" {
			continue
		}

		text := l.text
		if strings.HasPrefix(strings.TrimLeft(text, " "), "--- FAIL") {
			text = r.out.paint(ansiRed, text)
		}

		b.WriteString(r.prefix())
		b.WriteString(text)
		b.WriteByte('\n')
	}

	if !annotate {
		return
	}

	dir := r.pkgDir(p.path)

	for _, f := range r.failures {
		if f.file == "" || f.pkg != r.displayName(p.path) || (top != "" && topLevel(f.test) != top) {
			continue
		}

		r.out.annotation(b, path.Join(dir, f.file), f.line, f.test+" ["+r.spec.name+"]", f.msg)
	}
}

// releaseLines drops buffered lines that were rendered: all of them, or
// those of one top-level test.
func (r *run) releaseLines(p *pkgState, top string) {
	if top == "" {
		p.lines = nil

		return
	}

	kept := p.lines[:0]

	for _, l := range p.lines {
		if topLevel(l.test) != top {
			kept = append(kept, l)
		}
	}

	p.lines = kept
}

func counts(passed, skipped, failed int) string {
	if passed+skipped+failed == 0 {
		return "no tests"
	}

	s := fmt.Sprintf("%5d passed", passed)

	if skipped > 0 {
		s += fmt.Sprintf(" %4d skipped", skipped)
	}

	if failed > 0 {
		s += fmt.Sprintf(" %4d FAILED", failed)
	}

	return s
}

// startHeartbeat prints what is still running whenever a variant has been
// silent for the configured interval.
func (r *run) startHeartbeat(ctx context.Context) func() {
	if r.cfg.heartbeat <= 0 {
		return func() {}
	}

	done := make(chan struct{})
	stopped := make(chan struct{})

	go func() {
		defer close(stopped)

		t := time.NewTicker(r.cfg.heartbeat)
		defer t.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-t.C:
				r.heartbeat()
			}
		}
	}()

	return func() {
		close(done)
		<-stopped
	}
}

func (r *run) heartbeat() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if time.Since(r.lastPrint) < r.cfg.heartbeat {
		return
	}

	now := time.Now()
	items := make([]string, 0, 8)

	for _, p := range r.pkgs {
		if p.done || p.started.IsZero() {
			continue
		}

		if r.cfg.perTest {
			for name, ts := range p.tops {
				items = append(items, fmt.Sprintf("%s (%d subtests, %s)", name, ts.passed+ts.skipped+ts.failed, fmtSeconds(now.Sub(ts.started).Seconds())))
			}
		} else {
			items = append(items, fmt.Sprintf("%s (%s)", r.displayName(p.path), fmtSeconds(now.Sub(p.started).Seconds())))
		}
	}

	if len(items) == 0 {
		return
	}

	sort.Strings(items)

	r.lastPrint = now

	r.out.block(func(b *strings.Builder) {
		fmt.Fprintf(b, "%s… %s elapsed, running: %s\n", r.prefix(), fmtSeconds(now.Sub(r.firstEvent).Seconds()), strings.Join(items, ", "))
	})
}

// finish records the exit status and renders packages that never reported
// completion (a killed run) as failed.
func (r *run) finish(code int, waitErr error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.exitCode = code

	if waitErr != nil {
		r.exitErr = waitErr.Error()
	}

	for _, p := range r.pkgs {
		if p.done || len(p.lines) == 0 {
			continue
		}

		r.pkgDone(p, &testEvent{Action: actionFail, Package: p.path})
	}
}

func (r *run) ok() bool {
	return r.exitCode == 0 && r.failed == 0 && r.failedPkgs == 0
}

func (r *run) elapsed() float64 {
	if r.firstEvent.IsZero() {
		return 0
	}

	return r.lastEvent.Sub(r.firstEvent).Seconds()
}

// resultLine is the final one-line verdict of a run.
func (r *run) resultLine() string {
	verdict := r.out.paint(ansiGreen, "✓ PASSED")
	if !r.ok() {
		verdict = r.out.paint(ansiRed, "✗ FAILED")
	}

	s := fmt.Sprintf("%s%s  %d packages", r.prefix(), verdict, r.testedPkgs)

	if r.noTestPkgs > 0 {
		s += fmt.Sprintf(" (+%d without tests)", r.noTestPkgs)
	}

	s += fmt.Sprintf(" · %d passed · %d skipped · %d failed · %s", r.passed, r.skipped, r.failed, fmtSeconds(r.elapsed()))

	if r.exitCode != 0 {
		s += fmt.Sprintf(" · exit status %d", r.exitCode)
	}

	if r.stray > 0 {
		s += fmt.Sprintf(" · %d lines outside the event stream", r.stray)
	}

	return s
}
