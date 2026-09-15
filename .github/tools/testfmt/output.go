package main

import (
	"fmt"
	"io"
	"strings"
	"sync"
)

const (
	ansiReset  = "\x1b[0m"
	ansiRed    = "\x1b[31m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"

	// GitHub renders at most 10 annotations of each kind per step.
	maxAnnotations = 10
)

// printer owns stdout. Every block (a package line with its collapsed
// details, a failure excerpt, a heartbeat) is written in a single call so
// the concurrently running variants never interleave inside a block.
type printer struct {
	mu          sync.Mutex
	w           io.Writer
	github      bool
	color       bool
	nameWidth   int
	annotations int
}

func newPrinter(w io.Writer, github, color bool, names []string) *printer {
	width := 0
	for _, n := range names {
		if len(n) > width {
			width = len(n)
		}
	}

	return &printer{w: w, github: github, color: color, nameWidth: width}
}

// block builds one output block under the lock and writes it atomically.
func (p *printer) block(fn func(b *strings.Builder)) {
	p.mu.Lock()
	defer p.mu.Unlock()

	var b strings.Builder
	fn(&b)

	if b.Len() > 0 {
		_, _ = io.WriteString(p.w, b.String())
	}
}

// prefix returns the padded "[NAME] " tag that leads every line of a run.
func (p *printer) prefix(name string) string {
	return fmt.Sprintf("[%s]%*s", name, p.nameWidth-len(name)+1, "")
}

func (p *printer) paint(code, s string) string {
	if !p.color {
		return s
	}

	return code + s + ansiReset
}

// groupStart opens a collapsed block in the GitHub log. Outside GitHub the
// title is printed as a plain line and the body is skipped by the caller.
func (p *printer) groupStart(b *strings.Builder, title string) {
	if p.github {
		b.WriteString("::group::")
	}

	b.WriteString(title)
	b.WriteByte('\n')
}

func (p *printer) groupEnd(b *strings.Builder) {
	if p.github {
		b.WriteString("::endgroup::\n")
	}
}

// annotation emits a ::error annotation pointing at file:line, unless the
// per-step cap is reached. Must be called from inside block.
func (p *printer) annotation(b *strings.Builder, file string, line int, title, msg string) {
	if !p.github || p.annotations >= maxAnnotations {
		return
	}

	p.annotations++

	fmt.Fprintf(b, "::error file=%s,line=%d,title=%s::%s\n",
		escapeProperty(file), line, escapeProperty(title), escapeData(msg))
}

// escapeData escapes an annotation message per the workflow command format.
func escapeData(s string) string {
	return strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A").Replace(s)
}

// escapeProperty escapes an annotation property value.
func escapeProperty(s string) string {
	return strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A", ":", "%3A", ",", "%2C").Replace(s)
}

// fmtSeconds renders a duration the way go test does for short runs and
// as m/s beyond a minute.
func fmtSeconds(sec float64) string {
	if sec < 60 {
		return fmt.Sprintf("%.1fs", sec)
	}

	whole := int(sec + 0.5)

	return fmt.Sprintf("%dm%02ds", whole/60, whole%60)
}

// progressBar renders done/total as a fixed-width bar; empty without a total.
func progressBar(done, total int) string {
	const width = 20

	if total <= 0 {
		return ""
	}

	filled := done * width / total
	if filled > width {
		filled = width
	}

	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}
