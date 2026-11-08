package harness

// Entry describes one fixture type: how to construct a fresh Go value for it.
type Entry struct {
	// New returns a pointer to a fresh zero value of the Go mirror type.
	New func() any
}

// Registries maps fixture directory name -> fixture typeName -> entry.
// Type names collide across fixture directories (e.g. SampleShapeContainer),
// so registries are scoped per directory.
var Registries = map[string]map[string]Entry{
	"test_basic_types":              basicTypes,
	"test_merkleization_boundaries": boundaryTypes,
	"test_progressive_types":        progressiveTypes,
	"test_progressive_containers":   progContainerTypes,
	"test_compatible_unions":        unionTypes,
	"test_decode_failure_smoke":     smokeTypes,
	"test_extended_types":           extendedTypes,
}

// generatedCodeAvailable is overridden to true by an init() in generated_check.go,
// which dynssz-gen emits alongside generated_ssz.go.
var generatedCodeAvailable = false
