package harness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// Vector is one ssz-specs conformance test vector.
type Vector struct {
	TestID          string          // fixture key
	Dir             string          // fixture directory (type namespace)
	TypeName        string          `json:"typeName"`
	Value           json.RawMessage `json:"value"`
	Serialized      string          `json:"serialized"`
	Root            string          `json:"root"`
	RawBytes        string          `json:"rawBytes"`
	RejectionReason string          `json:"rejectionReason"`
}

// LoadFixtures walks the fixtures root (e.g. ~/repos/ssz-specs/fixtures/ssz/ssz)
// and returns all vectors.
func LoadFixtures(root string) ([]Vector, error) {
	var vectors []Vector
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(path) != ".json" {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var file map[string]Vector
		if err := json.Unmarshal(data, &file); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		dir := filepath.Base(filepath.Dir(path))
		for key, v := range file {
			v.TestID = key
			v.Dir = dir
			vectors = append(vectors, v)
		}
		return nil
	})
	return vectors, err
}

// FixturesRoot resolves the fixtures directory from $SSZ_FIXTURES or the
// default checkout location.
// FixturesRoot is the directory of ssz-specs `just fill` JSON vectors to
// replay (SSZ_FIXTURES), or "" when none is configured: the fixture phases are
// then skipped and only the generative phases run.
func FixturesRoot() string {
	return os.Getenv("SSZ_FIXTURES")
}

// loadFixturesOrNone loads the configured fixtures; without a configured
// directory it reports an empty set and logs that the fixture phase is off.
func loadFixturesOrNone(t *testing.T) []Vector {
	t.Helper()
	root := FixturesRoot()
	if root == "" {
		t.Log("SSZ_FIXTURES not set: fixture phase skipped (generative phase only)")
		return nil
	}
	vectors, err := LoadFixtures(root)
	if err != nil {
		t.Fatalf("load fixtures: %v", err)
	}
	if len(vectors) == 0 {
		t.Fatalf("no fixture vectors found under %s", root)
	}
	return vectors
}
