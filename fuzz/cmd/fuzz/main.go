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
	"runtime"
	"sync"
	"syscall"
	"time"

	dynssz "github.com/pk910/dynamic-ssz"
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
		workers    = flag.Int("workers", runtime.NumCPU(), "Number of parallel workers")
	)
	flag.Parse()

	if len(corpus.Registry) == 0 {
		log.Fatal("No types in corpus registry. Run 'go run ./fuzz/cmd/generate' first.")
	}

	if *seed == 0 {
		*seed = time.Now().UnixNano()
	}

	if *workers < 1 {
		*workers = 1
	}

	log.Printf("Starting fuzzer with %d types, seed=%d, workers=%d",
		len(corpus.Registry), *seed, *workers)
	log.Printf("Reports will be saved to: %s", *reportDir)

	reporter, err := engine.NewReporter(*reportDir, *maxIssues)
	if err != nil {
		log.Fatalf("Failed to create reporter: %v", err)
	}
	defer reporter.Close()

	// Create shared DynSsz instances (TypeCache is thread-safe via RWMutex).
	// Sharing avoids duplicating large type caches across workers.
	ds := dynssz.NewDynSsz(nil, dynssz.WithNoFastSsz())
	dsExt := dynssz.NewDynSsz(nil, dynssz.WithNoFastSsz(), dynssz.WithExtendedTypes())

	// Warm up type caches by doing one marshal per type.
	// This populates the cache before workers start, avoiding write contention.
	log.Println("Warming up type caches...")
	for _, entry := range corpus.Registry {
		instance := entry.New()
		target := ds
		if entry.Extended {
			target = dsExt
		}
		// SizeSSZ triggers type descriptor caching for the type and all nested types
		_, _ = target.SizeSSZ(instance)
	}
	log.Println("Cache warm-up complete")

	stats := &engine.Stats{}

	// Stop channel for coordinating shutdown
	stopCh := make(chan struct{})

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	if *duration > 0 {
		log.Printf("Will run for %s", *duration)
	}

	startTime := time.Now()
	statsTicker := time.NewTicker(*statsEvery)
	defer statsTicker.Stop()

	// Launch workers - each gets its own RNG but shares DynSsz instances
	var wg sync.WaitGroup
	for i := range *workers {
		wg.Add(1)
		workerSeed := *seed + int64(i)*6364136223846793005
		go func() {
			defer wg.Done()
			eng := engine.NewEngine(reporter, stats, ds, dsExt, workerSeed, *maxData)
			typeIdx := i // stagger starting positions
			for {
				select {
				case <-stopCh:
					return
				default:
					entry := corpus.Registry[typeIdx%len(corpus.Registry)]
					eng.FuzzEntry(entry)
					typeIdx++
				}
			}
		}()
	}

	log.Println("Fuzzing...")

	// Set up deadline channel
	var deadlineCh <-chan time.Time
	if *duration > 0 {
		deadlineCh = time.After(*duration)
	}

	// Main control loop
	for {
		select {
		case <-sigCh:
			fmt.Println()
			close(stopCh)
			wg.Wait()
			printFinalStats(stats, time.Since(startTime), reporter)
			return
		case <-deadlineCh:
			fmt.Println()
			close(stopCh)
			wg.Wait()
			printFinalStats(stats, time.Since(startTime), reporter)
			return
		case <-statsTicker.C:
			engine.PrintStats(stats, time.Since(startTime))
		}
	}
}

func printFinalStats(stats *engine.Stats, elapsed time.Duration, reporter *engine.Reporter) {
	engine.PrintStats(stats, elapsed)
	fmt.Println()
	fmt.Printf("\nFuzzing complete. %d unique issues found.\n", reporter.IssueCount())
}
