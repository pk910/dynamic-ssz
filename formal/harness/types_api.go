package harness

import "github.com/pk910/dynamic-ssz/sszutils"

// Types whose roots the public hasher helpers (PutUint64Array,
// PutRootVector) compute directly; see TestHasherAPILink.

// APIUint64List100 is List[uint64, 100].
type APIUint64List100 []uint64

var _ = sszutils.Annotate[APIUint64List100](`ssz-max:"100"`)

// APIRoots5 is Vector[Bytes32, 5].
type APIRoots5 [5][32]byte

// BitByte is a named byte: arrays of it are stored element-wise, not as a
// byte array, which takes the reflection engine's element-wise bitvector path.
type BitByte uint8

// APIBitvector12Elem is Bitvector[12] stored as two named bytes.
type APIBitvector12Elem [2]BitByte

var _ = sszutils.Annotate[APIBitvector12Elem](`ssz-type:"bitvector" ssz-bitsize:"12"`)

var apiTypes = map[string]Entry{
	"APIBitvector12Elem": {New: func() any { return new(APIBitvector12Elem) }},
	"APIUint64List100":   {New: func() any { return new(APIUint64List100) }},
	"APIRoots5":          {New: func() any { return new(APIRoots5) }},
}
