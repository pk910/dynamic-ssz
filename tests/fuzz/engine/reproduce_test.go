package engine

import (
	"bytes"
	"math/rand"
	"testing"

	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/tests/fuzz/corpus"
)

// TestAllTypesNoPanic verifies that no corpus type panics during
// fill + marshal + HTR with valid data across multiple random seeds.
func TestAllTypesNoPanic(t *testing.T) {
	ds := dynssz.NewDynSsz(nil, dynssz.WithNoFastSsz())
	dsExt := dynssz.NewDynSsz(nil, dynssz.WithNoFastSsz(), dynssz.WithExtendedTypes())

	for _, entry := range corpus.Registry {
		target := ds
		if entry.Extended {
			target = dsExt
		}

		t.Run(entry.Name, func(t *testing.T) {
			for seed := int64(0); seed < 20; seed++ {
				func() {
					defer func() {
						if r := recover(); r != nil {
							t.Fatalf("PANIC at seed %d: %v", seed, r)
						}
					}()

					rng := rand.New(rand.NewSource(seed))
					filler := NewFiller(rng)
					instance := entry.New()
					filler.FillStruct(instance)

					_, _ = target.HashTreeRoot(instance)
					_, _ = target.MarshalSSZ(instance)
				}()
			}
		})
	}
}

// TestStreamMarshalConsistency verifies that buffer and streaming marshal
// produce identical output for all corpus types with valid data.
func TestStreamMarshalConsistency(t *testing.T) {
	ds := dynssz.NewDynSsz(nil, dynssz.WithNoFastSsz())
	dsExt := dynssz.NewDynSsz(nil, dynssz.WithNoFastSsz(), dynssz.WithExtendedTypes())

	for _, entry := range corpus.Registry {
		target := ds
		if entry.Extended {
			target = dsExt
		}
		for seed := int64(0); seed < 20; seed++ {
			rng := rand.New(rand.NewSource(seed))
			filler := NewFiller(rng)
			instance := entry.New()
			filler.FillStruct(instance)

			bufBytes, err := target.MarshalSSZ(instance)
			if err != nil {
				continue
			}

			var streamBuf bytes.Buffer
			err = target.MarshalSSZWriter(instance, &streamBuf)
			if err != nil {
				continue
			}

			if !bytes.Equal(bufBytes, streamBuf.Bytes()) {
				sb := streamBuf.Bytes()
				for i := range bufBytes {
					if i >= len(sb) || bufBytes[i] != sb[i] {
						t.Errorf("%s seed %d: stream mismatch at byte %d: buffer=0x%02x stream=0x%02x (buflen=%d streamlen=%d)",
							entry.Name, seed, i, bufBytes[i], sb[i], len(bufBytes), len(sb))
						break
					}
				}
				break
			}
		}
	}
}
