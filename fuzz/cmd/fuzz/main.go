// Command fuzz runs the SSZ fuzzer against generated corpus types.
//
// It continuously tests reflection-based vs codegen-based SSZ implementations
// for consistency, checking marshal/unmarshal round-trips, hash tree roots,
// and streaming operations.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pk910/dynamic-ssz/fuzz/corpus"
	"github.com/pk910/dynamic-ssz/fuzz/engine"
)

func main() {
	var (
		seed       = flag.Int64("seed", 0, "Random seed (0 = time-based)")
		reportDir  = flag.String("report-dir", "fuzz-reports", "Directory for issue reports")
		maxData    = flag.Int("max-data", 4096, "Maximum random data length")
		maxIssues  = flag.Int("max-issues", 10000, "Maximum issues to persist (0 = unlimited)")
		duration   = flag.Duration("duration", 0, "Maximum run duration (0 = infinite)")
		statsEvery = flag.Duration("stats-every", 5*time.Second, "Stats print interval")
	)
	flag.Parse()

	if len(corpus.Registry) == 0 {
		log.Fatal("No types in corpus registry. Run 'go run ./fuzz/cmd/generate' first.")
	}

	if *seed == 0 {
		*seed = time.Now().UnixNano()
	}

	log.Printf("Starting fuzzer with %d types, seed=%d", len(corpus.Registry), *seed)
	log.Printf("Reports will be saved to: %s", *reportDir)

	reporter, err := engine.NewReporter(*reportDir, *maxIssues)
	if err != nil {
		log.Fatalf("Failed to create reporter: %v", err)
	}
	defer reporter.Close()

	eng := engine.NewEngine(reporter, *seed, *maxData)
	stats := eng.GetStats()

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Set up deadline if specified
	var deadline <-chan time.Time
	if *duration > 0 {
		deadline = time.After(*duration)
		log.Printf("Will run for %s", *duration)
	}

	startTime := time.Now()
	statsTicker := time.NewTicker(*statsEvery)
	defer statsTicker.Stop()

	typeIdx := 0

	log.Println("Fuzzing...")
	for {
		select {
		case <-sigCh:
			fmt.Println() // newline after stats
			printFinalStats(stats, time.Since(startTime), reporter)
			return
		case <-deadline:
			fmt.Println()
			printFinalStats(stats, time.Since(startTime), reporter)
			return
		case <-statsTicker.C:
			engine.PrintStats(stats, time.Since(startTime))
		default:
			// Fuzz next type
			entry := corpus.Registry[typeIdx]
			eng.FuzzEntry(entry)
			typeIdx = (typeIdx + 1) % len(corpus.Registry)
		}
	}
}

func printFinalStats(stats *engine.Stats, elapsed time.Duration, reporter *engine.Reporter) {
	engine.PrintStats(stats, elapsed)
	fmt.Println()
	fmt.Printf("\nFuzzing complete. %d unique issues found.\n", reporter.IssueCount())
}
