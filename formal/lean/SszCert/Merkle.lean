import SszCert.Basic
import SszCert.Bitlist
import SszCert.Bitvector
import SszCert.Sha256

/-!
# SSZ merkleization

The two tree shapes SSZ defines — the bounded binary tree (`merkleRoot`,
zero-subtree padding to a power-of-two width) and the progressive spine of
EIP-7916 (`progRoot`, right-leaning subtrees of 4^k chunks) — plus the three
mix-ins (length, active-fields word, selector).

Each root computation has an explicit *tree* counterpart (`merkleTree`,
`progTree`) that materializes the `Node` structure a proof/tree API walks,
with summary leaves for elided zero subtrees. The agreement theorems

* `root_merkleTree` — `(merkleTree d ns).root = merkleRoot d (ns.map .root)`
* `root_progTree`   — likewise for the progressive spine

are what make "the tree variant returns the same root as the direct
computation" a *theorem* for every certified type (the property whose
violation was dynssz issue #191: `GetTree` reordering leaves against
`HashTreeRoot`).

The hash function is never unfolded; no cryptographic property is assumed.
-/

namespace Ssz
namespace Merkle

/-- A 32-byte hash/chunk. -/
abbrev Hash := List UInt8

/-- Parent hash of two child hashes. -/
def hashPair (l r : Hash) : Hash := SHA256.sha256 (l ++ r)

/-- Root of the all-zero perfect subtree of depth `d`. -/
def zeroHash : Nat → Hash
  | 0 => List.replicate 32 0
  | d + 1 => hashPair (zeroHash d) (zeroHash d)

/-- Smallest depth `d` with `n ≤ 2^d` (the tree depth for a chunk
    capacity of `n`, i.e. `log2 (next_pow_of_two n)`). -/
def depthFor (n : Nat) : Nat :=
  if h : n ≤ 1 then 0 else 1 + depthFor ((n + 1) / 2)
decreasing_by omega

/-! ## Bounded merkleization -/

/-- SSZ Merkle root of `chunks` in a tree of depth `d`: pairwise hashing with
    virtual zero subtrees for the missing right part. Callers guarantee
    `chunks.length ≤ 2^d`. -/
def merkleRoot : Nat → List Hash → Hash
  | 0, [] => zeroHash 0
  | 0, c :: _ => c
  | d + 1, [] => zeroHash (d + 1)
  | d + 1, c :: cs =>
    hashPair (merkleRoot d ((c :: cs).take (2 ^ d)))
             (merkleRoot d ((c :: cs).drop (2 ^ d)))

/-- Explicit Merkle tree node. A `leaf` is either a data chunk or the summary
    of an elided all-zero subtree. -/
inductive Node where
  | leaf (h : Hash)
  | branch (l r : Node)

/-- The root a node hashes to. -/
def Node.root : Node → Hash
  | .leaf h => h
  | .branch l r => hashPair l.root r.root

/-- The explicit tree over subtree nodes that `merkleRoot` computes the root
    of: same shape, with the nodes hanging below and zero subtrees as summary
    leaves. -/
def merkleTree : Nat → List Node → Node
  | 0, [] => .leaf (zeroHash 0)
  | 0, n :: _ => n
  | d + 1, [] => .leaf (zeroHash (d + 1))
  | d + 1, n :: ns =>
    .branch (merkleTree d ((n :: ns).take (2 ^ d)))
            (merkleTree d ((n :: ns).drop (2 ^ d)))

/-- **Tree/root agreement (bounded)**: hashing the explicit tree gives
    exactly the direct Merkle root of the node roots. -/
theorem root_merkleTree (d : Nat) (ns : List Node) :
    (merkleTree d ns).root = merkleRoot d (ns.map Node.root) := by
  induction d generalizing ns with
  | zero => cases ns <;> rfl
  | succ d ih =>
    cases ns with
    | nil => rfl
    | cons n ns' =>
      simp only [merkleTree, Node.root, List.map_cons, merkleRoot]
      rw [← List.map_cons, ← List.map_take, ← List.map_drop, ih, ih]

/-! ## Progressive merkleization (EIP-7916) -/

/-- Progressive Merkle root: a right-leaning spine of bounded subtrees
    holding `4^k` chunks at spine level `k`, terminated by a zero leaf. The
    `fuel` bounds the recursion; any `fuel ≥ chunks.length` computes the
    spine (each level consumes at least one chunk). -/
def progRootF : Nat → Nat → List Hash → Hash
  | _, _, [] => zeroHash 0
  | 0, _, _ :: _ => zeroHash 0
  | fuel + 1, k, c :: cs =>
    hashPair (merkleRoot (2 * k) ((c :: cs).take (4 ^ k)))
             (progRootF fuel (k + 1) ((c :: cs).drop (4 ^ k)))

/-- Progressive Merkle root of a chunk sequence (spec entry point). -/
def progRoot (chunks : List Hash) : Hash :=
  progRootF chunks.length 0 chunks

/-- Explicit progressive tree over subtree nodes. -/
def progTreeF : Nat → Nat → List Node → Node
  | _, _, [] => .leaf (zeroHash 0)
  | 0, _, _ :: _ => .leaf (zeroHash 0)
  | fuel + 1, k, n :: ns =>
    .branch (merkleTree (2 * k) ((n :: ns).take (4 ^ k)))
            (progTreeF fuel (k + 1) ((n :: ns).drop (4 ^ k)))

/-- Explicit progressive tree of a node sequence. -/
def progTree (ns : List Node) : Node :=
  progTreeF ns.length 0 ns

theorem root_progTreeF (fuel k : Nat) (ns : List Node) :
    (progTreeF fuel k ns).root = progRootF fuel k (ns.map Node.root) := by
  induction fuel generalizing k ns with
  | zero => cases ns <;> rfl
  | succ fuel ih =>
    cases ns with
    | nil => rfl
    | cons n ns' =>
      simp only [progTreeF, Node.root, List.map_cons, progRootF]
      rw [← List.map_cons, ← List.map_take, ← List.map_drop, ih,
        root_merkleTree]

/-- **Tree/root agreement (progressive)**. -/
theorem root_progTree (ns : List Node) :
    (progTree ns).root = progRoot (ns.map Node.root) := by
  simp only [progTree, progRoot, List.length_map, root_progTreeF]

/-! ## Mix-ins and packing -/

/-- Mix a 32-byte word into a root (word on the right). -/
def mixIn (root w : Hash) : Hash := hashPair root w

/-- Length mix-in word: the count as a 32-byte little-endian integer. -/
def lengthWord (n : Nat) : Hash := encodeLE n 32

/-- Selector mix-in word (EIP-8016): zero-extended selector byte. -/
def selectorWord (sel : Nat) : Hash := encodeLE sel 32

/-- Active-fields mix-in word (EIP-7495): layout bits, lowest bit first. -/
def activeFieldsWord (layout : List Bool) : Hash :=
  encodeLE (Bitlist.bitsToNat layout) 32

/-- Zero-pad a partial chunk to 32 bytes. -/
def padChunk (bs : List UInt8) : Hash :=
  bs ++ List.replicate (32 - bs.length) 0

/-- Split serialized bytes into 32-byte chunks, zero-padding the tail. -/
def packBytes : List UInt8 → List Hash
  | [] => []
  | b :: bs =>
    padChunk ((b :: bs).take 32) :: packBytes ((b :: bs).drop 32)
termination_by bs => bs.length
decreasing_by simp [List.length_drop]; omega

/-- Pack a bit sequence LSB-first into chunks (no delimiter — bitlist
    sentinels are a serialization artifact, not a merkleization one). -/
def packBits (bits : List Bool) : List Hash :=
  packBytes (Bitvector.serializeBitvec bits)

/-- Chunk capacity of a byte capacity. -/
def byteChunks (byteLimit : Nat) : Nat := (byteLimit + 31) / 32

/-- Chunk capacity of a bit capacity. -/
def bitChunks (bitLimit : Nat) : Nat := (bitLimit + 255) / 256

/-- Chunk leaves of packed data, as nodes. -/
def chunkNodes (chunks : List Hash) : List Node := chunks.map Node.leaf

theorem map_root_chunkNodes (chunks : List Hash) :
    (chunkNodes chunks).map Node.root = chunks := by
  have h : Node.root ∘ Node.leaf = id := rfl
  simp [chunkNodes, List.map_map, h]

/-- Tree/root agreement for packed leaves. -/
theorem root_merkleTree_chunks (d : Nat) (chunks : List Hash) :
    (merkleTree d (chunkNodes chunks)).root = merkleRoot d chunks := by
  rw [root_merkleTree, map_root_chunkNodes]

/-- Tree/root agreement for packed leaves, progressive. -/
theorem root_progTree_chunks (chunks : List Hash) :
    (progTree (chunkNodes chunks)).root = progRoot chunks := by
  rw [root_progTree, map_root_chunkNodes]

end Merkle
end Ssz
