import EthLib.Serialization.Ssz

/-! Layout equality of SSZ declarations: equal up to field names.

Field names are labels (JSON keys, path steps); serialization, roots and
proofs depend only on the layout. The cross-check against EthLib's typed
consensus containers (`EthCross.lean`) compares layouts, since dynamic-ssz
declarations carry Go field names and EthLib's carry the specification's.

One more equivalence is admitted: a byte-aligned `Bitvector[8·m]` and a
`ByteVector[m]` have the same layout. Both encode as `m` bytes accepting
every byte string (no padding bits exist to be checked) and root as the same
chunks of those bytes; Go mirrors often declare such bitfields as byte
arrays.

And, outside list and vector elements, `uint(w)` and `ByteVector[w]`: both
encode as the same `w` little-endian bytes accepting every byte string, and
a field roots as its single zero-padded chunk either way. As elements they
differ (basic values pack into shared chunks), so there the match is exact. -/

namespace SszSpec

open EthLib.Serialization.Ssz

mutual
/-- `sameLayout`, knowing whether the declarations are list/vector elements. -/
def sameLayoutAt (elem : Bool) : Desc → Desc → Bool
  | .bool, .bool => true
  | .uint a, .uint b => a == b
  | .byteVector a, .byteVector b => a == b
  | .byteList a, .byteList b => a == b
  | .bitVector a, .bitVector b => a == b
  | .bitVector a, .byteVector b => a == 8 * b
  | .byteVector a, .bitVector b => 8 * a == b
  | .uint a, .byteVector b => !elem && a == b
  | .byteVector a, .uint b => !elem && a == b
  | .bitList a, .bitList b => a == b
  | .progressiveBitList a, .progressiveBitList b => a == b
  | .vector a n, .vector b m => sameLayoutAt true a b && n == m
  | .list a n, .list b m => sameLayoutAt true a b && n == m
  | .progressiveList a n, .progressiveList b m => sameLayoutAt true a b && n == m
  | .container _ fs, .container _ gs => sameLayouts fs gs
  | .progressiveContainer l _ fs, .progressiveContainer k _ gs => l == k && sameLayouts fs gs
  | .compatibleUnion s os, .compatibleUnion t ps => s == t && sameLayouts os ps
  | _, _ => false
termination_by structural a => a

/-- Pointwise layout equality of two field lists of equal length. -/
def sameLayouts : List Desc → List Desc → Bool
  | [], [] => true
  | a :: as, b :: bs => sameLayoutAt false a b && sameLayouts as bs
  | _, _ => false
termination_by structural as => as
end

/-- Whether two declarations have the same layout (names ignored). -/
def sameLayout (a b : Desc) : Bool := sameLayoutAt false a b

end SszSpec
