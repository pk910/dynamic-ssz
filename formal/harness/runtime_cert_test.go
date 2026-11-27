package harness

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"testing"

	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/formal/certify/certlib"
	"github.com/pk910/dynamic-ssz/ssztypes"
	"gopkg.in/yaml.v2"
)

// FORMAL_UPDATE=1 rewrites the generated Ethereum certificate files instead
// of checking them.
var updateCerts = os.Getenv("FORMAL_UPDATE") == "1"

// presets are the consensus presets the Ethereum types are certified for.
var presets = []string{"mainnet", "minimal"}

func loadPreset(t testing.TB, name string) map[string]any {
	raw, err := os.ReadFile(filepath.Join("..", "..", "tests", "spectests", "presets", name+"-preset.yaml"))
	if err != nil {
		t.Fatalf("read preset %s: %v", name, err)
	}
	var specs map[string]any
	if err := yaml.Unmarshal(raw, &specs); err != nil {
		t.Fatalf("parse preset %s: %v", name, err)
	}
	return specs
}

// runtimeDescriptor is the descriptor the reflection engine itself uses for
// the type behind newFn, with resolved spec values.
func runtimeDescriptor(t testing.TB, ds *dynssz.DynSsz, newFn func() any) *ssztypes.TypeDescriptor {
	typ := reflect.TypeOf(newFn()).Elem()
	desc, err := ds.GetTypeCache().GetTypeDescriptor(typ, nil, nil, nil)
	if err != nil {
		t.Fatalf("descriptor for %v: %v", typ, err)
	}
	return desc
}

func reflectionInstance(specs map[string]any) *dynssz.DynSsz {
	return dynssz.NewDynSsz(specs, dynssz.WithNoDelegation(), dynssz.WithExtendedTypes())
}

var defRe = regexp.MustCompile(`(?m)^def ([CD])_([A-Za-z0-9_]+) : (?:Codec|Desc) :=\n  (.*)$`)

func committedDefs(t testing.TB, path string) map[string]string {
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	defs := map[string]string{}
	for _, m := range defRe.FindAllStringSubmatch(string(raw), -1) {
		defs[m[2]] = m[3]
	}
	return defs
}

// TestRuntimeCertificatesAgree is the cross-check of dynamic-ssz's two
// descriptor builders: the certificates in the repository are written from
// the go/types descriptor dynssz-gen consumes (dynssz-certify); here every
// certified type is written down again from the reflection engine's runtime
// descriptor, and both terms must be identical in both models.
func TestRuntimeCertificatesAgree(t *testing.T) {
	cert := committedDefs(t, filepath.Join("..", "lean", "SszCert", "Certified.lean"))
	spec := committedDefs(t, filepath.Join("..", "spec", "SszSpec", "Certified.lean"))
	ds := reflectionInstance(nil)
	for _, sub := range certifiedSubjects(t) {
		desc := runtimeDescriptor(t, ds, sub.newFn)
		expr, err := certlib.CertExpr(desc)
		if err != nil {
			t.Errorf("%s: runtime certificate: %v", sub.name, err)
			continue
		}
		if expr != cert[sub.name] {
			t.Errorf("DESCRIPTOR BUILDERS DISAGREE on %s (SszCert):\n  go/types: %s\n  runtime:  %s", sub.name, cert[sub.name], expr)
		}
		sexpr, err := certlib.SpecExpr(desc)
		switch {
		case errors.Is(err, certlib.ErrNoSpec):
			if _, ok := spec[sub.name]; ok {
				t.Errorf("%s: declared in the spec but the runtime descriptor has no spec type", sub.name)
			}
		case err != nil:
			t.Errorf("%s: runtime spec declaration: %v", sub.name, err)
		case sexpr != spec[sub.name]:
			t.Errorf("DESCRIPTOR BUILDERS DISAGREE on %s (EthLib):\n  go/types: %s\n  runtime:  %s", sub.name, spec[sub.name], sexpr)
		}
	}
}

// ethCertName names the certificate of a Go type under a preset.
func ethCertName(preset, goName string) string { return preset + "_" + goName }

// TestEthCertificatesFresh certifies the Ethereum consensus types
// (tests/spectests/codegen, phase0 to gloas) under both presets from the
// reflection engine's descriptors with the preset's spec values resolved,
// into formal/lean/SszCert/Eth.lean and formal/spec/SszSpec/Eth*.lean, and
// requires the committed files to match (FORMAL_UPDATE=1 rewrites them).
func TestEthCertificatesFresh(t *testing.T) {
	names := make([]string, 0, len(ethGoTypes))
	for name := range ethGoTypes {
		names = append(names, name)
	}
	sort.Strings(names)
	var certs []certlib.CertEntry
	var specs []certlib.SpecEntry
	for _, preset := range presets {
		ds := reflectionInstance(loadPreset(t, preset))
		for _, name := range names {
			desc := runtimeDescriptor(t, ds, ethGoTypes[name])
			cn := ethCertName(preset, name)
			expr, err := certlib.CertExpr(desc)
			if err != nil {
				t.Fatalf("%s: %v", cn, err)
			}
			certs = append(certs, certlib.CertEntry{PkgName: "codegen", Name: cn, Expr: expr})
			sexpr, err := certlib.SpecExpr(desc)
			if err != nil {
				t.Fatalf("%s (spec): %v", cn, err)
			}
			specs = append(specs, certlib.SpecEntry{PkgName: "codegen", Name: cn, Expr: sexpr})
		}
	}
	certFile := filepath.Join("..", "lean", "SszCert", "Eth.lean")
	descFile := filepath.Join("..", "spec", "SszSpec", "Eth.lean")
	thmFile := filepath.Join("..", "spec", "SszSpec", "EthTheorems.lean")
	dir := t.TempDir()
	outCert, outDesc, outThm := certFile, descFile, thmFile
	if !updateCerts {
		outCert, outDesc, outThm = filepath.Join(dir, "a"), filepath.Join(dir, "b"), filepath.Join(dir, "c")
	}
	if err := certlib.WriteCertFileAs(certs, outCert, "certifiedEth"); err != nil {
		t.Fatal(err)
	}
	if err := certlib.WriteSpecFilesAs(specs, outDesc, outThm, "declaredEth", "SszSpec.Eth"); err != nil {
		t.Fatal(err)
	}
	if updateCerts {
		t.Logf("wrote %d Ethereum certificates", len(certs))
		return
	}
	for _, pair := range [][2]string{{certFile, outCert}, {descFile, outDesc}, {thmFile, outThm}} {
		want, err := os.ReadFile(pair[1])
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(pair[0])
		if err != nil || string(got) != string(want) {
			t.Errorf("%s is stale (regenerate with FORMAL_UPDATE=1 go test -run TestEthCertificatesFresh)", pair[0])
		}
	}
}
