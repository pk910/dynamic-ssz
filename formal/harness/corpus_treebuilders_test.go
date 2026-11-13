package harness

import (
	"encoding/hex"
	"reflect"
	"testing"

	"github.com/pk910/dynamic-ssz/formal/harness/typegen"
	"github.com/pk910/dynamic-ssz/ssztypes"
	"github.com/pk910/dynamic-ssz/treeproof"
	"pgregory.net/rapid"
)

// chunksOf splits bytes into 32-byte chunks, the last zero-padded.
func chunksOf(b []byte) [][]byte {
	var out [][]byte
	for i := 0; i < len(b); i += 32 {
		c := make([]byte, 32)
		copy(c, b[i:min(i+32, len(b))])
		out = append(out, c)
	}
	return out
}

func leavesOf(chunks [][]byte) []*treeproof.Node {
	out := make([]*treeproof.Node, len(chunks))
	for i, c := range chunks {
		out[i] = treeproof.NewNodeWithValue(c)
	}
	return out
}

// TestCorpusTreeBuilders holds treeproof's public tree builders to the
// engines: for every corpus root whose merkleization is a plain or mixed-in
// tree over the chunks of its own serialization — byte vectors and lists,
// vectors and lists of basic elements, bit vectors — the tree built by
// TreeFromChunks / TreeFromNodes / TreeFromNodesWithMixin (and their 64-bit
// forms) from those chunks must hash to the root both engines (and, through
// the link, the models) compute.
func TestCorpusTreeBuilders(t *testing.T) {
	entries := corpusEntries(t)
	eng := newEngineSet(typegen.Specs())
	built := 0
	for _, e := range entries {
		if e.Recursive || e.Opaque {
			continue
		}
		rt := reflect.TypeOf(e.New()).Elem()
		desc, err := eng.refl.GetTypeCache().GetTypeDescriptor(rt, nil, nil, nil)
		if err != nil {
			continue
		}
		var mode string
		switch desc.SszType {
		case ssztypes.SszVectorType:
			if desc.ElemDesc != nil && desc.ElemDesc.SszType.IsBasic() && desc.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic == 0 {
				mode = "vector"
			}
		case ssztypes.SszBitvectorType:
			mode = "vector"
		case ssztypes.SszListType:
			if desc.ElemDesc != nil && desc.ElemDesc.SszType.IsBasic() && desc.SszTypeFlags&ssztypes.SszTypeFlagHasLimit != 0 {
				mode = "list"
			}
		default:
		}
		if mode == "" {
			continue
		}
		e := e
		t.Run("tree/"+e.Name, func(t *testing.T) {
			rapid.Check(t, func(rt *rapid.T) {
				v := subject{name: e.Name, newFn: e.New}.genValue(t, rt, eng)
				enc, ok := eng.encodePaths(failer(t, "tree/"+e.Name), v)
				if !ok {
					return
				}
				want, err := eng.gen.HashTreeRoot(v)
				if err != nil {
					t.Fatalf("%s: HashTreeRoot: %v", e.Name, err)
				}
				root := hex.EncodeToString(want[:])
				chunks := chunksOf(enc)
				check := func(builder string, node *treeproof.Node, err error) {
					if err != nil {
						t.Errorf("TREE BUILDER %s %s: %v", e.Name, builder, err)
						return
					}
					if got := hex.EncodeToString(node.Hash()); got != root {
						t.Errorf("TREE BUILDER DIVERGENCE %s %s: built %s, engines %s", e.Name, builder, got, root)
					}
				}
				switch mode {
				case "vector":
					// a fixed vector of basic elements: the tree over its chunks,
					// padded to the power of two the builders take (as fastssz)
					padded := chunks
					for len(padded)&(len(padded)-1) != 0 || len(padded) == 0 {
						padded = append(padded, make([]byte, 32))
					}
					n, err := treeproof.TreeFromChunks(padded)
					check("TreeFromChunks", n, err)
					n, err = treeproof.TreeFromNodes(leavesOf(chunks), len(padded))
					check("TreeFromNodes", n, err)
					n, err = treeproof.TreeFromNodes64(leavesOf(chunks), uint64(len(padded)))
					check("TreeFromNodes64", n, err)
				case "list":
					elemSize := uint64(desc.ElemDesc.Size)
					limitChunks := (desc.Limit*elemSize + 31) / 32
					num := uint64(len(enc)) / elemSize
					if limitChunks <= uint64(1<<31) {
						n, err := treeproof.TreeFromNodesWithMixin(leavesOf(chunks), int(num), int(limitChunks))
						check("TreeFromNodesWithMixin", n, err)
					}
					n, err := treeproof.TreeFromNodesWithMixin64(leavesOf(chunks), num, limitChunks)
					check("TreeFromNodesWithMixin64", n, err)
				}
				built++
			})
		})
	}
	t.Logf("tree builders: %d trees built and compared", built)
}
