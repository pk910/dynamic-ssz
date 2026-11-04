package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

const (
	slowestShown = 8
	maxMsgLen    = 160
)

// writeSummary appends the markdown step summary: a verdict table, the
// failures, the slowest packages and the skipped tests grouped by reason.
func writeSummary(cfg *config, runs []*run) error {
	f, err := os.OpenFile(cfg.summary, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644) //nolint:gosec // the path is $GITHUB_STEP_SUMMARY or the operator's -summary flag
	if err != nil {
		return err
	}

	defer func() { _ = f.Close() }()

	_, err = f.WriteString(renderSummary(cfg, runs))

	return err
}

func renderSummary(cfg *config, runs []*run) string {
	var b strings.Builder

	fmt.Fprintf(&b, "## %s\n\n", cfg.title)
	b.WriteString("| run | packages | tests | passed | skipped | failed | time | result |\n")
	b.WriteString("|:--|--:|--:|--:|--:|--:|--:|:--|\n")

	for _, r := range runs {
		result := "✅ passed"
		if !r.ok() {
			result = "❌ failed"

			if r.exitCode != 0 {
				result += fmt.Sprintf(" (exit %d)", r.exitCode)
			}
		}

		pkgs := fmt.Sprint(r.testedPkgs)
		if r.noTestPkgs > 0 {
			pkgs += fmt.Sprintf(" (+%d w/o tests)", r.noTestPkgs)
		}

		fmt.Fprintf(&b, "| %s | %s | %d | %d | %d | %d | %s | %s |\n",
			r.spec.name, pkgs, r.passed+r.skipped+r.failed, r.passed, r.skipped, r.failed, fmtSeconds(r.elapsed()), result)
	}

	b.WriteByte('\n')

	writeFailures(&b, runs)
	writeSlowest(&b, runs)
	writeSkips(&b, runs)

	return b.String()
}

func writeFailures(b *strings.Builder, runs []*run) {
	n := 0
	for _, r := range runs {
		n += len(r.failures)
	}

	if n == 0 {
		return
	}

	fmt.Fprintf(b, "### Failures (%d)\n\n", n)
	b.WriteString("| run | package | test | location | message |\n")
	b.WriteString("|:--|:--|:--|:--|:--|\n")

	for _, r := range runs {
		for _, f := range r.failures {
			loc := ""
			if f.file != "" {
				loc = fmt.Sprintf("%s:%d", f.file, f.line)
			}

			fmt.Fprintf(b, "| %s | `%s` | `%s` | %s | %s |\n", r.spec.name, f.pkg, f.test, loc, cell(f.msg))
		}
	}

	b.WriteByte('\n')
}

func writeSlowest(b *strings.Builder, runs []*run) {
	type entry struct {
		run string
		pkgTime
	}

	all := make([]entry, 0, 64)
	for _, r := range runs {
		for _, t := range r.times {
			all = append(all, entry{run: r.spec.name, pkgTime: t})
		}
	}

	if len(all) == 0 {
		return
	}

	sort.SliceStable(all, func(i, j int) bool { return all[i].elapsed > all[j].elapsed })

	if len(all) > slowestShown {
		all = all[:slowestShown]
	}

	b.WriteString("<details><summary>Slowest packages</summary>\n\n")
	b.WriteString("| run | package | time |\n|:--|:--|--:|\n")

	for _, e := range all {
		fmt.Fprintf(b, "| %s | `%s` | %s |\n", e.run, e.pkg, fmtSeconds(e.elapsed))
	}

	b.WriteString("\n</details>\n\n")
}

func writeSkips(b *strings.Builder, runs []*run) {
	type group struct {
		reason string
		tests  map[string]struct{}
		runs   []string
	}

	byReason := make(map[string]*group, 16)
	total := 0

	for _, r := range runs {
		for _, s := range r.skips {
			total++

			g, ok := byReason[s.reason]
			if !ok {
				g = &group{reason: s.reason, tests: make(map[string]struct{}, 8), runs: make([]string, 0, len(runs))}
				byReason[s.reason] = g
			}

			g.tests[s.pkg+"."+s.test] = struct{}{}

			if len(g.runs) == 0 || g.runs[len(g.runs)-1] != r.spec.name {
				g.runs = append(g.runs, r.spec.name)
			}
		}
	}

	if total == 0 {
		return
	}

	groups := make([]*group, 0, len(byReason))
	for _, g := range byReason {
		groups = append(groups, g)
	}

	sort.Slice(groups, func(i, j int) bool {
		if len(groups[i].tests) != len(groups[j].tests) {
			return len(groups[i].tests) > len(groups[j].tests)
		}

		return groups[i].reason < groups[j].reason
	})

	fmt.Fprintf(b, "<details><summary>Skipped tests (%d)</summary>\n\n", total)
	b.WriteString("| reason | tests | runs |\n|:--|--:|:--|\n")

	for _, g := range groups {
		reason := g.reason
		if reason == "" {
			reason = "(no reason given)"
		}

		names := make([]string, 0, len(g.tests))
		for t := range g.tests {
			names = append(names, t)
		}

		sort.Strings(names)

		fmt.Fprintf(b, "| %s | %d | %s |\n", cell(reason), len(g.tests), strings.Join(g.runs, ", "))

		for _, t := range names {
			fmt.Fprintf(b, "| &nbsp;&nbsp;`%s` | | |\n", t)
		}
	}

	b.WriteString("\n</details>\n\n")
}

// cell makes free text safe inside a markdown table cell.
func cell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")

	if len(s) > maxMsgLen {
		s = s[:maxMsgLen] + "…"
	}

	return s
}
