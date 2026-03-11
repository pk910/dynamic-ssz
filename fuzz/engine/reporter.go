package engine

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// IssueType classifies the kind of fuzzing issue found.
type IssueType string

const (
	IssuePanic           IssueType = "panic"
	IssueMarshalMismatch IssueType = "marshal-mismatch"
	IssueHTRMismatch     IssueType = "htr-mismatch"
	IssueStreamMismatch  IssueType = "stream-mismatch"
	IssueUnmarshalDiff   IssueType = "unmarshal-diff"
)

// Issue represents a single fuzzing issue found.
type Issue struct {
	Type             IssueType
	TypeName         string
	Data             []byte
	Details          string
	ReflectionOutput []byte
	CodegenOutput    []byte
}

// Reporter persists fuzzing issues to disk.
type Reporter struct {
	dir       string
	mu        sync.Mutex
	issueID   int
	logFile   *os.File
	dedup     map[string]bool
	maxIssues int
}

// NewReporter creates a new issue reporter that writes to the given directory.
func NewReporter(dir string, maxIssues int) (*Reporter, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create report dir: %w", err)
	}

	logPath := filepath.Join(dir, "fuzz.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}

	return &Reporter{
		dir:       dir,
		logFile:   logFile,
		dedup:     make(map[string]bool, 256),
		maxIssues: maxIssues,
	}, nil
}

// Close closes the reporter's log file.
func (r *Reporter) Close() error {
	return r.logFile.Close()
}

// Report persists an issue to disk.
func (r *Reporter) Report(issue Issue) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Dedup by type+typename+details
	dedupKey := fmt.Sprintf("%s:%s:%s", issue.Type, issue.TypeName, issue.Details)
	if r.dedup[dedupKey] {
		return
	}
	r.dedup[dedupKey] = true

	if r.maxIssues > 0 && r.issueID >= r.maxIssues {
		return
	}

	r.issueID++
	issueDir := filepath.Join(r.dir, fmt.Sprintf("issue-%05d-%s", r.issueID, issue.Type))

	if err := os.MkdirAll(issueDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create issue dir: %v\n", err)
		return
	}

	// Write input data
	if err := os.WriteFile(filepath.Join(issueDir, "input.bin"), issue.Data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write input: %v\n", err)
	}

	// Write details
	details := fmt.Sprintf(
		"Issue #%d\nType: %s\nTypeName: %s\nTimestamp: %s\nInput length: %d\nInput hex: %s\n\nDetails:\n%s\n",
		r.issueID,
		issue.Type,
		issue.TypeName,
		time.Now().UTC().Format(time.RFC3339),
		len(issue.Data),
		hex.EncodeToString(issue.Data),
		issue.Details,
	)

	if err := os.WriteFile(filepath.Join(issueDir, "details.txt"), []byte(details), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write details: %v\n", err)
	}

	// Write outputs if present
	if issue.ReflectionOutput != nil {
		os.WriteFile(filepath.Join(issueDir, "reflection_output.bin"), issue.ReflectionOutput, 0644)
	}
	if issue.CodegenOutput != nil {
		os.WriteFile(filepath.Join(issueDir, "codegen_output.bin"), issue.CodegenOutput, 0644)
	}

	// Append to log
	logEntry := fmt.Sprintf("[%s] #%d %s %s: %s\n",
		time.Now().UTC().Format(time.RFC3339),
		r.issueID,
		issue.Type,
		issue.TypeName,
		issue.Details,
	)
	fmt.Fprint(r.logFile, logEntry)
	fmt.Fprintf(os.Stderr, "\n*** ISSUE #%d: %s in %s: %s\n", r.issueID, issue.Type, issue.TypeName, issue.Details)
}

// IssueCount returns the number of unique issues reported.
func (r *Reporter) IssueCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.issueID
}
