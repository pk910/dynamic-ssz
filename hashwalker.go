package dynssz

// HashWalker is our own interface that mirrors fastssz.HashWalker
// This allows us to avoid importing fastssz directly while still being
// compatible with types that implement HashTreeRootWith
type HashWalker interface {
	// Hash returns the latest hash generated during merkleize
	Hash() []byte

	// Methods for appending single values
	AppendUint8(i uint8)
	AppendUint32(i uint32)
	AppendUint64(i uint64)
	AppendBytes32(b []byte)

	// Methods for putting values into the buffer
	PutUint64Array(b []uint64, maxCapacity ...uint64)
	PutUint64(i uint64)
	PutUint32(i uint32)
	PutUint16(i uint16)
	PutUint8(i uint8)
	PutBitlist(bb []byte, maxSize uint64)
	PutBool(b bool)
	PutBytes(b []byte)

	// Buffer manipulation methods
	FillUpTo32()
	Append(i []byte)
	Index() int

	// Merkleization methods
	Merkleize(indx int)
	MerkleizeWithMixin(indx int, num, limit uint64)
}
