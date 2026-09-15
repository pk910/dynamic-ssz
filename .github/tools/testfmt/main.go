// testfmt runs one or more `go test` variants concurrently and renders their
// -json event streams as a readable GitHub Actions log: one line per
// completed package, failures in the open with file/line annotations, a
// heartbeat for long silent stretches, a one-line verdict per variant, then
// the verbose output as one collapsed group per package, and a markdown
// step summary. Outside GitHub Actions it prints the same package and
// failure lines without the collapsed details.
//
// Usage:
//
//	testfmt [flags] -cmd 'NAME: [KEY=VALUE ...] go test ARGS...' [-cmd ...]
//
// Flags of the go test command must use the -flag=value form; trailing
// arguments that do not start with a dash are the package patterns.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	goCmd     = "go"
	goTestCmd = "test"
)

type config struct {
	module    string
	workDir   string
	logDir    string
	summary   string
	title     string
	perTest   bool
	heartbeat time.Duration
	github    bool
	color     bool
}

type runSpec struct {
	name     string
	env      []string
	argv     []string
	patterns []string
}

func main() {
	os.Exit(realMain())
}

func realMain() int {
	cfg, specs, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "testfmt:", err)

		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	names := make([]string, 0, len(specs))
	for _, s := range specs {
		names = append(names, s.name)
	}

	out := newPrinter(os.Stdout, cfg.github, cfg.color, names)

	runs := make([]*run, 0, len(specs))
	errs := make([]error, len(specs))

	var wg sync.WaitGroup

	for i := range specs {
		r := newRun(&specs[i], cfg, out)
		runs = append(runs, r)

		wg.Add(1)

		go func(i int, r *run) {
			defer wg.Done()

			errs[i] = r.execute(ctx)
		}(i, r)
	}

	wg.Wait()

	failed := false

	out.block(func(b *strings.Builder) {
		for i, r := range runs {
			b.WriteString(r.resultLine())
			b.WriteByte('\n')

			if errs[i] != nil {
				fmt.Fprintf(b, "%s%s\n", r.prefix(), out.paint(ansiRed, errs[i].Error()))

				failed = true
			}

			if !r.ok() {
				failed = true
			}
		}
	})

	// The collapsed groups come last: streamed live they would render
	// expanded in the GitHub log viewer until the step completes.
	for _, r := range runs {
		out.block(r.writeDetails)
	}

	if cfg.summary != "" {
		if err := writeSummary(cfg, runs); err != nil {
			fmt.Fprintln(os.Stderr, "testfmt: step summary:", err)
		}
	}

	if failed {
		return 1
	}

	return 0
}

func parseArgs(args []string) (*config, []runSpec, error) {
	cfg := &config{}
	specs := make([]runSpec, 0, 4)

	fs := flag.NewFlagSet("testfmt", flag.ContinueOnError)
	fs.StringVar(&cfg.logDir, "log-dir", "", "directory receiving one verbose NAME.log per command (default: none)")
	fs.StringVar(&cfg.summary, "summary", os.Getenv("GITHUB_STEP_SUMMARY"), "markdown file the step summary is appended to")
	fs.StringVar(&cfg.title, "title", "Tests", "heading of the step summary")
	fs.StringVar(&cfg.module, "module", "", "module path stripped from package names (default: from go.mod)")
	fs.DurationVar(&cfg.heartbeat, "heartbeat", 30*time.Second, "print what is still running after this much silence (0 disables)")
	fs.BoolVar(&cfg.perTest, "per-test", false, "report top-level tests instead of packages (for one package with many subtests)")
	fs.BoolVar(&cfg.github, "github", os.Getenv("GITHUB_ACTIONS") == "true", "emit GitHub Actions groups and annotations")

	root := fs.String("root", os.Getenv("GITHUB_WORKSPACE"), "repository root, for annotation paths (default: current directory)")
	color := fs.String("color", "auto", "colorize output: auto, always, never")

	fs.Func("cmd", "'NAME: [KEY=VALUE ...] go test ARGS...' (repeatable)", func(s string) error {
		spec, err := parseSpec(s)
		if err != nil {
			return err
		}

		specs = append(specs, spec)

		return nil
	})

	if err := fs.Parse(args); err != nil {
		return nil, nil, err
	}

	if len(specs) == 0 {
		return nil, nil, errors.New("at least one -cmd is required")
	}

	if cfg.module == "" {
		cfg.module = moduleFromGoMod()
	}

	if *root != "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, nil, err
		}

		rel, err := filepath.Rel(*root, cwd)
		if err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			cfg.workDir = filepath.ToSlash(rel)
		}
	}

	switch *color {
	case "always":
		cfg.color = true
	case "never":
		cfg.color = false
	case "auto":
		cfg.color = cfg.github || stdoutIsTerminal()
	default:
		return nil, nil, fmt.Errorf("invalid -color %q", *color)
	}

	if cfg.logDir != "" {
		if err := os.MkdirAll(cfg.logDir, 0o755); err != nil {
			return nil, nil, err
		}
	}

	return cfg, specs, nil
}

// parseSpec splits "NAME: KEY=VALUE ... go test ARGS..." into its parts and
// injects -json into the go test command.
func parseSpec(s string) (runSpec, error) {
	name, rest, ok := strings.Cut(s, ":")
	name = strings.TrimSpace(name)

	if !ok || name == "" {
		return runSpec{}, fmt.Errorf("-cmd %q: expected 'NAME: go test ...'", s)
	}

	fields := strings.Fields(rest)
	spec := runSpec{name: name, env: make([]string, 0, 4), patterns: make([]string, 0, 2)}

	i := 0
	for ; i < len(fields); i++ {
		key, _, isAssign := strings.Cut(fields[i], "=")
		if !isAssign || !isEnvKey(key) {
			break
		}

		spec.env = append(spec.env, fields[i])
	}

	cmd := fields[i:]
	if len(cmd) < 2 || cmd[0] != goCmd || cmd[1] != goTestCmd {
		return runSpec{}, fmt.Errorf("-cmd %q: command must be 'go test ...'", s)
	}

	spec.argv = make([]string, 0, len(cmd)+1)
	spec.argv = append(spec.argv, goCmd, goTestCmd, "-json")
	spec.argv = append(spec.argv, cmd[2:]...)

	for _, a := range cmd[2:] {
		if !strings.HasPrefix(a, "-") {
			spec.patterns = append(spec.patterns, a)
		}
	}

	return spec, nil
}

func isEnvKey(s string) bool {
	if s == "" || (s[0] >= '0' && s[0] <= '9') {
		return false
	}

	for _, c := range s {
		if c != '_' && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			return false
		}
	}

	return true
}

func moduleFromGoMod() string {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		return ""
	}

	for _, line := range strings.Split(string(data), "\n") {
		if mod, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.TrimSpace(mod)
		}
	}

	return ""
}

func stdoutIsTerminal() bool {
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}

	return info.Mode()&os.ModeCharDevice != 0
}
