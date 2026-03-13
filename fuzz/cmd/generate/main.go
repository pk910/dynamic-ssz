// Command generate creates random SSZ-compatible Go types, runs codegen on them,
// and produces a compilable corpus for the fuzzer.
package main

import (
	"flag"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pk910/dynamic-ssz/fuzz/typegen"
)

func main() {
	var (
		numTypes     = flag.Int("num-types", 50, "Number of types to generate")
		maxFields    = flag.Int("max-fields", 8, "Maximum fields per struct")
		maxDepth     = flag.Int("max-depth", 3, "Maximum nesting depth")
		maxArrayLen  = flag.Int("max-array-len", 8, "Maximum fixed array length")
		maxListLimit = flag.Int("max-list-limit", 32, "Maximum list ssz-max")
		extended     = flag.Bool("extended", false, "Include extended types")
		seed         = flag.Int64("seed", 0, "Random seed (0 = time-based)")
		corpusDir    = flag.String("corpus-dir", "", "Corpus output directory (default: ./fuzz/corpus)")
	)
	flag.Parse()

	if *corpusDir == "" {
		// Find project root relative to this command
		*corpusDir = findCorpusDir()
	}

	if *seed == 0 {
		*seed = int64(os.Getpid()) ^ int64(os.Getuid())
	}

	cfg := typegen.Config{
		NumTypes:     *numTypes,
		MaxFields:    *maxFields,
		MaxDepth:     *maxDepth,
		MaxArrayLen:  *maxArrayLen,
		MaxListLimit: *maxListLimit,
		Extended:     *extended,
		Seed:         *seed,
	}

	log.Printf("Generating %d types (seed=%d, extended=%v)", cfg.NumTypes, cfg.Seed, cfg.Extended)

	gen := typegen.NewGenerator(cfg)
	types := gen.Generate()

	log.Printf("Generated %d types total (including helper types)", len(types))

	// Ensure corpus directory exists
	if err := os.MkdirAll(*corpusDir, 0755); err != nil {
		log.Fatalf("Failed to create corpus dir: %v", err)
	}

	// Clean previous generated files
	cleanGeneratedFiles(*corpusDir)

	// Write type definitions
	typesSource := gen.WriteGoSource("corpus")
	typesPath := filepath.Join(*corpusDir, "types_gen.go")
	if err := os.WriteFile(typesPath, []byte(typesSource), 0644); err != nil {
		log.Fatalf("Failed to write types: %v", err)
	}
	log.Printf("Wrote type definitions to %s", typesPath)

	// Write registry
	registrySource := gen.WriteRegistry("corpus")
	registryPath := filepath.Join(*corpusDir, "registry_gen.go")
	if err := os.WriteFile(registryPath, []byte(registrySource), 0644); err != nil {
		log.Fatalf("Failed to write registry: %v", err)
	}
	log.Printf("Wrote registry to %s", registryPath)

	// Collect type names for codegen
	typeNames := make([]string, 0, len(types))
	for _, td := range types {
		typeNames = append(typeNames, td.Name)
	}

	// Run codegen via dynssz-gen
	codegenOutput := filepath.Join(*corpusDir, "codegen_gen.go")
	if err := runCodegen(*corpusDir, typeNames, codegenOutput, *extended); err != nil {
		log.Fatalf("Code generation failed: %v", err)
	}
	log.Printf("Wrote generated SSZ code to %s", codegenOutput)

	// Verify compilation
	if err := verifyCompilation(*corpusDir); err != nil {
		log.Fatalf("Compilation verification failed: %v", err)
	}

	log.Printf("Successfully generated and verified corpus with %d types", len(types))
}

func findCorpusDir() string {
	// Walk up to find go.mod
	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, "fuzz", "corpus")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			log.Fatal("Could not find project root (go.mod)")
		}
		dir = parent
	}
}

func cleanGeneratedFiles(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), "_gen.go") {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

func runCodegen(corpusDir string, typeNames []string, outputFile string, extended bool) error {
	// Find project root for dynssz-gen
	projectRoot := filepath.Dir(filepath.Dir(corpusDir))

	args := []string{
		"run", filepath.Join(projectRoot, "dynssz-gen"),
		"-package", "./fuzz/corpus",
		"-package-name", "corpus",
		"-types", strings.Join(typeNames, ","),
		"-output", outputFile,
		"-without-fastssz",
		"-with-streaming",
		"-v",
	}

	if extended {
		args = append(args, "-with-extended-types")
	}

	cmd := exec.Command("go", args...)
	cmd.Dir = projectRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func verifyCompilation(corpusDir string) error {
	projectRoot := filepath.Dir(filepath.Dir(corpusDir))

	cmd := exec.Command("go", "build", "./fuzz/corpus/...")
	cmd.Dir = projectRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
