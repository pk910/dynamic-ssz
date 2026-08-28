package engine

// oracleChecks is the deep-oracle battery layered on top of the reflection-vs-
// codegen differential. For a filled valid instance it runs, in one pass:
//
//   - native-hash HTR: the value's HashTreeRoot on the native Go sha256 backend
//     must equal the accelerated (hashtree) backend — a third hashing engine
//     beyond reflection-vs-codegen.
//   - SizeSSZ == len(MarshalSSZ): the size oracle must match the encoded length.
//   - reference oracle: an independent, first-principles reflection SSZ
//     implementation (oracle_ref.go) that shares no code with dynssz, for the
//     classic-SSZ subset it supports — catches reflection==codegen-but-both-wrong.
//   - tree generation + proofs (proofs.go): GetTree()==HTR on both engines,
//     structural tree equality, and single/multi proof completeness + soundness.
//   - HashTreeRootWith on a caller-supplied hasher, fresh and reused-after-Reset.
//   - metamorphic HTR: two distinct serializations of the same type must not
//     share a root.
//   - determinism: HashTreeRoot is stable across repeats.

import (
	"bytes"
	"fmt"
	"reflect"

	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/hasher"
	"github.com/pk910/dynamic-ssz/tests/fuzz/corpus"
)

func (e *Engine) oracleChecks(entry corpus.TypeEntry, ds *dynssz.DynSsz, instance any) {
	report := func(it IssueType, kind, detail string, data []byte) {
		e.reporter.Report(&Issue{
			Type:     it,
			TypeName: fmt.Sprintf("%s [%s]", entry.Name, kind),
			Data:     data,
			Details:  detail,
		})
	}
	ck := func() { e.stats.OracleChecks.Add(1) }

	// Primary reflection root + bytes via the public API on the reflection ds.
	var refRoot [32]byte
	var rerr error
	if p := capturePanic(func() { refRoot, rerr = ds.HashTreeRoot(instance) }); p != "" {
		e.stats.Panics.Add(1)
		report(IssuePanic, "oracle-htr", p, nil)
		return
	}
	if rerr != nil {
		return
	}
	var encoded []byte
	var merr error
	if p := capturePanic(func() { encoded, merr = ds.MarshalSSZ(instance) }); p != "" {
		e.stats.Panics.Add(1)
		report(IssuePanic, "oracle-marshal", p, nil)
		return
	}

	// --- native-hash HTR cross-check ---
	nds := e.dsNative
	if entry.Extended {
		nds = e.dsNativeExt
	}
	if nds != nil {
		var nRoot [32]byte
		var nerr error
		if p := capturePanic(func() { nRoot, nerr = nds.HashTreeRoot(instance) }); p == "" && nerr == nil && nRoot != refRoot {
			e.stats.HTRMismatches.Add(1)
			report(IssueHTRMismatch, "htr-fast-vs-native", fmt.Sprintf("fast=%x native=%x", refRoot, nRoot), encoded)
		}
		ck()
	}

	// --- SizeSSZ == len(MarshalSSZ) ---
	if merr == nil {
		var sz int
		var serr error
		if p := capturePanic(func() { sz, serr = ds.SizeSSZ(instance) }); p == "" && serr == nil && sz != len(encoded) {
			report(IssueSizeMismatch, "size", fmt.Sprintf("SizeSSZ=%d len(marshal)=%d", sz, len(encoded)), encoded)
		}
		ck()
	}

	// --- independent reference oracle (supported classic-SSZ subset) ---
	if merr == nil && e.reference {
		if detail, ok := refCheck(reflect.TypeOf(instance), instance, encoded, refRoot); !ok {
			report(IssueReferenceMismatch, "reference-oracle", detail, encoded)
		}
		ck()
	}

	// --- tree generation + proof battery ---
	if e.proofs {
		cgds := e.dsCg
		if entry.Extended {
			cgds = e.dsCgExt
		}
		proofCheck(ds, cgds, instance, refRoot, e.rng,
			func(kind, detail string) { report(IssueProofFail, kind, detail, encoded) }, ck)
	}

	// --- G5: HashTreeRootWith on a caller-supplied hasher (fresh + reuse) ---
	hh := hasher.NewHasher()
	if p := capturePanic(func() {
		if err := ds.HashTreeRootWith(instance, hh); err == nil {
			if r, e2 := hh.HashRoot(); e2 == nil && r != refRoot {
				e.stats.HTRMismatches.Add(1)
				report(IssueHTRMismatch, "hashtreerootwith", fmt.Sprintf("with=%x htr=%x", r, refRoot), encoded)
			}
		}
	}); p != "" {
		e.stats.Panics.Add(1)
		report(IssuePanic, "hashtreerootwith", p, encoded)
	}
	ck()

	// second value for hasher reuse + metamorphic HTR
	val2 := entry.New()
	if capturePanic(func() { e.filler.FillStruct(val2) }) == "" {
		r2, e2 := ds.HashTreeRoot(val2)
		if e2 == nil {
			hh.Reset()
			if p := capturePanic(func() {
				if err := ds.HashTreeRootWith(val2, hh); err == nil {
					if r, e3 := hh.HashRoot(); e3 == nil && r != r2 {
						e.stats.HTRMismatches.Add(1)
						report(IssueHTRMismatch, "hashtreerootwith-reuse", fmt.Sprintf("reuse=%x fresh=%x", r, r2), encoded)
					}
				}
			}); p != "" {
				e.stats.Panics.Add(1)
				report(IssuePanic, "hashtreerootwith-reuse", p, encoded)
			}
			ck()
			// G10 metamorphic: distinct serialization ⇒ distinct root
			if b2, err := ds.MarshalSSZ(val2); err == nil && !bytes.Equal(encoded, b2) && refRoot == r2 {
				report(IssueMetamorphic, "metamorphic", fmt.Sprintf("distinct serializations share root %x", refRoot), encoded)
			}
			ck()
		}
	}

	// --- determinism: HTR stable across repeats ---
	for i := 0; i < 3; i++ {
		if r, e2 := ds.HashTreeRoot(instance); e2 == nil && r != refRoot {
			report(IssueNonDeterministic, "determinism", fmt.Sprintf("iter%d %x != %x", i, r, refRoot), encoded)
			break
		}
	}
	ck()
}
