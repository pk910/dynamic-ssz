package dynssz

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"hash"
	"math/bits"
	"sync"

	"github.com/prysmaticlabs/gohashtree"
)

// this hasher implementation was copied from the fastssz package
// https://github.com/ferranbt/fastssz/blob/main/hasher.go
// code has been modified for dynamis-ssz needs

// Compile-time check to ensure Hasher implements HashWalker interface
var _ HashWalker = (*Hasher)(nil)

var (
	// ErrIncorrectByteSize means that the byte size is incorrect
	ErrIncorrectByteSize = fmt.Errorf("incorrect byte size")

	// ErrIncorrectListSize means that the size of the list is incorrect
	ErrIncorrectListSize = fmt.Errorf("incorrect list size")
)

// DefaultHasherPool is a default hasher pool
var DefaultHasherPool HasherPool
var FastHasherPool HasherPool = HasherPool{
	HashFn: gohashtree.HashByteSlice,
}

var hasherInitialized bool
var hasherInitMutex sync.Mutex
var zeroHashes [65][32]byte
var zeroHashLevels map[string]int
var trueBytes, falseBytes, zeroBytes []byte

func initHasher() {
	hasherInitMutex.Lock()
	defer hasherInitMutex.Unlock()

	if hasherInitialized {
		return
	}

	hasherInitialized = true
	falseBytes = make([]byte, 32)
	trueBytes = make([]byte, 32)
	zeroBytes = make([]byte, 32)
	trueBytes[0] = 1
	zeroHashLevels = make(map[string]int)
	zeroHashLevels[string(falseBytes)] = 0

	tmp := [64]byte{}
	for i := 0; i < 64; i++ {
		copy(tmp[:32], zeroHashes[i][:])
		copy(tmp[32:], zeroHashes[i][:])
		zeroHashes[i+1] = sha256.Sum256(tmp[:])
		zeroHashLevels[string(zeroHashes[i+1][:])] = i + 1
	}
}

type HashFn func(dst []byte, input []byte) error

// NativeHashWrapper wraps a hash.Hash function into a HashFn
func NativeHashWrapper(hashFn hash.Hash) HashFn {
	return func(dst []byte, input []byte) error {
		hash := func(dst []byte, src []byte) {
			hashFn.Write(src[:32])
			hashFn.Write(src[32:64])
			hashFn.Sum(dst)
			hashFn.Reset()
		}

		layerLen := len(input) / 32
		if layerLen%2 == 1 {
			layerLen++
		}
		for i := 0; i < layerLen; i += 2 {
			hash(input[(i/2)*32:][:0], input[i*32:])
		}
		return nil
	}
}

// HasherPool may be used for pooling Hashers for similarly typed SSZs.
type HasherPool struct {
	HashFn HashFn
	pool   sync.Pool
}

// Get acquires a Hasher from the pool.
func (hh *HasherPool) Get() *Hasher {
	h := hh.pool.Get()
	if h == nil {
		if hh.HashFn == nil {
			return NewHasher()
		} else {
			return NewHasherWithHashFn(hh.HashFn)
		}
	}
	return h.(*Hasher)
}

// Put releases the Hasher to the pool.
func (hh *HasherPool) Put(h *Hasher) {
	h.Reset()
	hh.pool.Put(h)
}

// Hasher is a utility tool to hash SSZ structs
type Hasher struct {
	// buffer array to store hashing values
	buf []byte

	// tmp array used for uint64 and bitlist processing
	tmp []byte

	// sha256 hash function
	hash HashFn
}

// NewHasher creates a new Hasher object with sha256 hash
func NewHasher() *Hasher {
	return NewHasherWithHash(sha256.New())
}

// NewHasherWithHash creates a new Hasher object with a custom hash.Hash function
func NewHasherWithHash(hh hash.Hash) *Hasher {
	return NewHasherWithHashFn(NativeHashWrapper(hh))
}

// NewHasherWithHashFn creates a new Hasher object with a custom HashFn function
func NewHasherWithHashFn(hh HashFn) *Hasher {
	if !hasherInitialized {
		initHasher()
	}

	return &Hasher{
		hash: hh,
		tmp:  make([]byte, 32),
	}
}

// Reset resets the Hasher obj
func (h *Hasher) Reset() {
	h.buf = h.buf[:0]
}

func (h *Hasher) AppendBytes32(b []byte) {
	h.buf = append(h.buf, b...)
	if rest := len(b) % 32; rest != 0 {
		// pad zero bytes to the left
		h.buf = append(h.buf, zeroBytes[:32-rest]...)
	}
}

// PutUint64 appends a uint64 in 32 bytes
func (h *Hasher) PutUint64(i uint64) {
	binary.LittleEndian.PutUint64(h.tmp[:8], i)
	h.AppendBytes32(h.tmp[:8])
}

// PutUint32 appends a uint32 in 32 bytes
func (h *Hasher) PutUint32(i uint32) {
	binary.LittleEndian.PutUint32(h.tmp[:4], i)
	h.AppendBytes32(h.tmp[:4])
}

// PutUint16 appends a uint16 in 32 bytes
func (h *Hasher) PutUint16(i uint16) {
	binary.LittleEndian.PutUint16(h.tmp[:2], i)
	h.AppendBytes32(h.tmp[:2])
}

// PutUint8 appends a uint8 in 32 bytes
func (h *Hasher) PutUint8(i uint8) {
	h.tmp[0] = byte(i)
	h.AppendBytes32(h.tmp[:1])
}

func CalculateLimit(maxCapacity, numItems, size uint64) uint64 {
	limit := (maxCapacity*size + 31) / 32
	if limit != 0 {
		return limit
	}
	if numItems == 0 {
		return 1
	}
	return numItems
}

func (h *Hasher) FillUpTo32() {
	// pad zero bytes to the left
	if rest := len(h.buf) % 32; rest != 0 {
		h.buf = append(h.buf, zeroBytes[:32-rest]...)
	}
}

func (h *Hasher) AppendUint8(i uint8) {
	h.buf = marshalUint8(h.buf, i)
}

func (h *Hasher) AppendUint16(i uint16) {
	h.buf = marshalUint16(h.buf, i)
}

func (h *Hasher) AppendUint32(i uint32) {
	h.buf = marshalUint32(h.buf, i)
}

func (h *Hasher) AppendUint64(i uint64) {
	h.buf = marshalUint64(h.buf, i)
}

func (h *Hasher) Append(i []byte) {
	h.buf = append(h.buf, i...)
}

// PutRootVector appends an array of roots
func (h *Hasher) PutRootVector(b [][]byte, maxCapacity ...uint64) error {
	indx := h.Index()
	for _, i := range b {
		if len(i) != 32 {
			return fmt.Errorf("bad root")
		}
		h.buf = append(h.buf, i...)
	}

	if len(maxCapacity) == 0 {
		h.Merkleize(indx)
	} else {
		numItems := uint64(len(b))
		limit := CalculateLimit(maxCapacity[0], numItems, 32)

		h.MerkleizeWithMixin(indx, numItems, limit)
	}
	return nil
}

// PutUint64Array appends an array of uint64
func (h *Hasher) PutUint64Array(b []uint64, maxCapacity ...uint64) {
	indx := h.Index()
	for _, i := range b {
		h.AppendUint64(i)
	}

	// pad zero bytes to the left
	h.FillUpTo32()

	if len(maxCapacity) == 0 {
		// Array with fixed size
		h.Merkleize(indx)
	} else {
		numItems := uint64(len(b))
		limit := CalculateLimit(maxCapacity[0], numItems, 8)

		h.MerkleizeWithMixin(indx, numItems, limit)
	}
}

func parseBitlist(dst, buf []byte) ([]byte, uint64) {
	msb := uint8(bits.Len8(buf[len(buf)-1])) - 1
	size := uint64(8*(len(buf)-1) + int(msb))

	dst = append(dst, buf...)
	dst[len(dst)-1] &^= uint8(1 << msb)

	newLen := len(dst)
	for i := len(dst) - 1; i >= 0; i-- {
		if dst[i] != 0x00 {
			break
		}
		newLen = i
	}
	res := dst[:newLen]
	return res, size
}

// PutBitlist appends a ssz bitlist
func (h *Hasher) PutBitlist(bb []byte, maxSize uint64) {
	var size uint64
	h.tmp, size = parseBitlist(h.tmp[:0], bb)

	// merkleize the content with mix in length
	indx := h.Index()
	h.AppendBytes32(h.tmp)
	h.MerkleizeWithMixin(indx, size, (maxSize+255)/256)
}

// PutBool appends a boolean
func (h *Hasher) PutBool(b bool) {
	if b {
		h.buf = append(h.buf, trueBytes...)
	} else {
		h.buf = append(h.buf, falseBytes...)
	}
}

// PutBytes appends bytes
func (h *Hasher) PutBytes(b []byte) {
	if len(b) <= 32 {
		h.AppendBytes32(b)
		return
	}

	// if the bytes are longer than 32 we have to
	// merkleize the content
	indx := h.Index()
	h.AppendBytes32(b)
	h.Merkleize(indx)
}

// Index marks the current buffer index
func (h *Hasher) Index() int {
	return len(h.buf)
}

// Merkleize is used to merkleize the last group of the hasher
func (h *Hasher) Merkleize(indx int) {
	// merkleizeImpl will expand the `input` by 32 bytes if some hashing depth
	// hits an odd chunk length. But if we're at the end of `h.buf` already,
	// appending to `input` will allocate a new buffer, *not* expand `h.buf`,
	// so the next invocation will realloc, over and over and over. We can pre-
	// emptively cater for that by ensuring that an extra 32 bytes is always
	// available.
	if len(h.buf) == cap(h.buf) {
		h.buf = append(h.buf, zeroBytes...)
		h.buf = h.buf[:len(h.buf)-len(zeroBytes)]
	}
	input := h.buf[indx:]

	// merkleize the input
	input = h.merkleizeImpl(input[:0], input, 0)
	h.buf = append(h.buf[:indx], input...)
}

// MerkleizeWithMixin is used to merkleize the last group of the hasher
func (h *Hasher) MerkleizeWithMixin(indx int, num, limit uint64) {
	h.FillUpTo32()
	input := h.buf[indx:]

	// merkleize the input
	input = h.merkleizeImpl(input[:0], input, limit)

	// mixin with the size
	output := h.tmp[:32]
	for indx := range output {
		output[indx] = 0
	}
	marshalUint64(output[:0], num)
	input = append(input, output...)

	// input is of the form [<input><size>] of 64 bytes
	h.hash(input, input)
	h.buf = append(h.buf[:indx], input[:32]...)
}

func (h *Hasher) Hash() []byte {
	return h.buf[len(h.buf)-32:]
}

// HashRoot creates the hash final hash root
func (h *Hasher) HashRoot() (res [32]byte, err error) {
	if len(h.buf) != 32 {
		err = fmt.Errorf("expected 32 byte size")
		return
	}
	copy(res[:], h.buf)
	return
}

func (h *Hasher) nextPowerOfTwo(v uint64) uint {
	v--
	v |= v >> 1
	v |= v >> 2
	v |= v >> 4
	v |= v >> 8
	v |= v >> 16
	v++
	return uint(v)
}

func (h *Hasher) getDepth(d uint64) uint8 {
	if d <= 1 {
		return 0
	}
	i := h.nextPowerOfTwo(d)
	return 64 - uint8(bits.LeadingZeros(i)) - 1
}

func (h *Hasher) merkleizeImpl(dst []byte, input []byte, limit uint64) []byte {
	// count is the number of 32 byte chunks from the input, after right-padding
	// with zeroes to the next multiple of 32 bytes when the input is not aligned
	// to a multiple of 32 bytes.
	count := uint64((len(input) + 31) / 32)
	if limit == 0 {
		limit = count
	} else if count > limit {
		panic(fmt.Sprintf("BUG: count '%d' higher than limit '%d'", count, limit))
	}

	if limit == 0 {
		return append(dst, zeroBytes...)
	}
	if limit == 1 {
		if count == 1 {
			return append(dst, input[:32]...)
		}
		return append(dst, zeroBytes...)
	}

	depth := h.getDepth(limit)
	if len(input) == 0 {
		return append(dst, zeroHashes[depth][:]...)
	}

	for i := uint8(0); i < depth; i++ {
		layerLen := len(input) / 32
		oddNodeLength := layerLen%2 == 1

		if oddNodeLength {
			// is odd length
			input = append(input, zeroHashes[i][:]...)
			layerLen++
		}

		outputLen := (layerLen / 2) * 32

		h.hash(input, input)
		input = input[:outputLen]
	}

	return append(dst, input...)
}
