package engine

import (
	"fmt"

	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/hasher"
	"github.com/pk910/dynamic-ssz/tests/fuzz/corpus"
)

func (e *Engine) oracleChecks(entry corpus.TypeEntry, ds *dynssz.DynSsz, instance any) {
	report := func(issueType IssueType, kind, detail string, data []byte) {
		e.reporter.Report(&Issue{
			Type: issueType, TypeName: entry.Name + " [" + kind + "]",
			Details: detail, Data: data,
		})
	}

	encoded, err := ds.MarshalSSZ(instance)
	if err != nil {
		return
	}
	root, err := ds.HashTreeRoot(instance)
	if err != nil {
		return
	}

	e.stats.OracleChecks.Add(1)
	if size, err := ds.SizeSSZ(instance); err == nil && size != len(encoded) {
		report(IssueSizeMismatch, "size", fmt.Sprintf("SizeSSZ=%d len(MarshalSSZ)=%d", size, len(encoded)), encoded)
	}

	if e.reference {
		detail, supported, ok := referenceCheck(instance, encoded, root)
		if supported {
			e.stats.ReferenceChecks.Add(1)
			if !ok {
				report(IssueReferenceMismatch, "reference", detail, encoded)
			}
		} else {
			e.stats.ReferenceSkips.Add(1)
		}
	}

	native := e.dsNative
	if entry.Extended {
		native = e.dsNativeExtended
	}
	if native != nil {
		if nativeRoot, err := native.HashTreeRoot(instance); err == nil && nativeRoot != root {
			e.stats.HTRMismatches.Add(1)
			report(IssueHTRMismatch, "fast-vs-native", fmt.Sprintf("fast=%x native=%x", root, nativeRoot), encoded)
		}
	}

	hh := hasher.NewHasher()
	if err := ds.HashTreeRootWith(instance, hh); err == nil {
		if suppliedRoot, err := hh.HashRoot(); err == nil && suppliedRoot != root {
			e.stats.HTRMismatches.Add(1)
			report(IssueHTRMismatch, "HashTreeRootWith", fmt.Sprintf("public=%x supplied=%x", root, suppliedRoot), encoded)
		}
	}
	for i := 0; i < 2; i++ {
		if repeated, err := ds.HashTreeRoot(instance); err == nil && repeated != root {
			report(IssueNonDeterministic, "determinism", fmt.Sprintf("initial=%x repeat-%d=%x", root, i, repeated), encoded)
			break
		}
	}
}
