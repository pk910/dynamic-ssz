"""Differential-testing corpus generator.

Generates random SSZ values and byte-level mutations, classifies every input
with the executable Python spec (ethereum/ssz-specs), and emits vectors in the
ssz-specs fixture JSON format. The Go harness replays the corpus against
dynamic-ssz: the spec's accept/reject/value/root verdicts are the oracle.

Run from the ssz-specs checkout so its virtualenv provides the `ssz` package:

    cd ~/repos/ssz-specs && uv run --group test python \
        ~/projects/dynamic-ssz-formal-verification/src/oracle/gen_corpus.py \
        --out ~/projects/dynamic-ssz-formal-verification/results/oracle-corpus \
        --seed 873767 --per-type 10 --mutations 25
"""

from __future__ import annotations

import argparse
import json
import random
from pathlib import Path
from typing import Any, Callable, ClassVar

from ssz import (
    BitList,
    BitVector,
    Boolean,
    ByteList,
    ByteVector,
    CompatibleUnion,
    Container,
    List,
    ProgressiveBitList,
    ProgressiveContainer,
    ProgressiveList,
    Uint8,
    Uint16,
    Uint32,
    Uint64,
    Uint128,
    Uint256,
    Vector,
)
from ssz.ssz_base import SSZModel
from ssz.merkleization import hash_tree_root


# ---------------------------------------------------------------------------
# Schema definitions mirroring tests/fillers/ssz/*.py (and the Go harness).
# ---------------------------------------------------------------------------

# --- test_basic_types ---


class Bytes4(ByteVector):
    LENGTH = 4


class Bytes32(ByteVector):
    LENGTH = 32


class Bytes52(ByteVector):
    LENGTH = 52


class Bytes64(ByteVector):
    LENGTH = 64


class ByteList512KiB(ByteList):
    LIMIT = 512 * 1024


class SampleBitVector8(BitVector):
    LENGTH: ClassVar[int] = 8


class SampleBitVector64(BitVector):
    LENGTH: ClassVar[int] = 64


class SampleBitList16(BitList):
    LIMIT: ClassVar[int] = 16


class SampleUint16Vector3(Vector[Uint16]):
    LENGTH: ClassVar[int] = 3


class SampleUint64Vector4(Vector[Uint64]):
    LENGTH: ClassVar[int] = 4


class SampleUint32List16(List[Uint32]):
    LIMIT: ClassVar[int] = 16
    ELEMENT_TYPE = Uint32


class SampleBytes32List8(List[Bytes32]):
    LIMIT: ClassVar[int] = 8
    ELEMENT_TYPE = Bytes32


# --- test_merkleization_boundaries ---


class BoundaryBitVector1(BitVector):
    LENGTH: ClassVar[int] = 1


class BoundaryBitVector7(BitVector):
    LENGTH: ClassVar[int] = 7


class BoundaryBitVector9(BitVector):
    LENGTH: ClassVar[int] = 9


class BoundaryBitVector255(BitVector):
    LENGTH: ClassVar[int] = 255


class BoundaryBitVector256(BitVector):
    LENGTH: ClassVar[int] = 256


class BoundaryBitVector257(BitVector):
    LENGTH: ClassVar[int] = 257


class BoundaryBitList256(BitList):
    LIMIT: ClassVar[int] = 256


class BoundaryUint64List32(List[Uint64]):
    LIMIT: ClassVar[int] = 32
    ELEMENT_TYPE = Uint64


# --- test_progressive_types ---


class SampleUint64ProgressiveList(ProgressiveList[Uint64]):
    ELEMENT_TYPE = Uint64


class SampleBytes32ProgressiveList(ProgressiveList[Bytes32]):
    ELEMENT_TYPE = Bytes32


class SampleUint16ProgressiveList(ProgressiveList[Uint16]):
    ELEMENT_TYPE = Uint16


class SampleNestedProgressiveList(ProgressiveList[SampleUint16ProgressiveList]):
    ELEMENT_TYPE = SampleUint16ProgressiveList


class SampleContainerWithProgressiveList(Container):
    a: Uint16
    b: SampleUint64ProgressiveList
    c: Uint8


# --- test_progressive_containers ---


class SampleUint16List4(List[Uint16]):
    LIMIT = 4
    ELEMENT_TYPE = Uint16


class PCSquare(ProgressiveContainer):
    ACTIVE_FIELDS = (1, 0, 1)
    side: Uint16
    color: Uint8


class PCCircle(ProgressiveContainer):
    ACTIVE_FIELDS = (0, 1, 1)
    radius: Uint16
    color: Uint8


class SampleOneField(ProgressiveContainer):
    ACTIVE_FIELDS = (1,)
    a: Uint16


class SampleLeadingGaps(ProgressiveContainer):
    ACTIVE_FIELDS = (0, 0, 1)
    c: Uint32


class SampleMultipleGaps(ProgressiveContainer):
    ACTIVE_FIELDS = (1, 0, 0, 1, 0, 1)
    a: Uint8
    b: Uint16
    c: Uint32


class SampleWidestLayout(ProgressiveContainer):
    ACTIVE_FIELDS = (*([0] * 255), 1)
    tail: Uint8


class SampleLevelBoundary(ProgressiveContainer):
    ACTIVE_FIELDS = (1, *([0] * 20), 1)
    first: Uint16
    last: Uint8


class SampleBoundedListField(ProgressiveContainer):
    ACTIVE_FIELDS = (1, 0, 1)
    head: Uint64
    body: SampleUint16List4


class SampleProgressiveFields(ProgressiveContainer):
    ACTIVE_FIELDS = (1, 1, 1)
    head: Uint64
    numbers: SampleUint64ProgressiveList
    flags: ProgressiveBitList


class SampleInnerShape(ProgressiveContainer):
    ACTIVE_FIELDS = (1, 0, 1)
    x: Uint16
    y: Uint8


class SampleOuterShape(ProgressiveContainer):
    ACTIVE_FIELDS = (1, 0, 1)
    head: Uint8
    inner: SampleInnerShape


class PCSquareProgressiveList(ProgressiveList[PCSquare]):
    ELEMENT_TYPE = PCSquare


class PCShapeContainer(Container):
    tag: Uint8
    shape: PCSquare


# --- test_compatible_unions ---


class CUSquareProgressiveList(ProgressiveList[PCSquare]):
    ELEMENT_TYPE = PCSquare


class CUCircleProgressiveList(ProgressiveList[PCCircle]):
    ELEMENT_TYPE = PCCircle


class SampleShape(CompatibleUnion):
    OPTIONS = {1: PCSquare, 2: PCCircle, 127: PCSquare}


class SampleNumbers(CompatibleUnion):
    OPTIONS = {1: SampleUint16List4, 2: SampleUint16List4}


class SampleEmptyProne(CompatibleUnion):
    OPTIONS = {1: CUSquareProgressiveList, 2: CUCircleProgressiveList}


class SampleSquareOnly(CompatibleUnion):
    OPTIONS = {5: PCSquare}


class SampleNestedShape(CompatibleUnion):
    OPTIONS = {1: SampleShape, 2: SampleSquareOnly}


class CUShapeContainer(Container):
    tag: Uint64
    body: SampleShape


class CUShapeProgressiveContainer(ProgressiveContainer):
    ACTIVE_FIELDS = (1, 0, 1)
    tag: Uint64
    body: SampleShape


class CUShapeProgressiveList(ProgressiveList[SampleShape]):
    ELEMENT_TYPE = SampleShape


# --- test_decode_failure_smoke ---


class SmokeBitList8(BitList):
    LIMIT: ClassVar[int] = 8


# ---------------------------------------------------------------------------
# Random value generators.
# ---------------------------------------------------------------------------

PROGRESSIVE_CAP = 120

rng = random.Random(0)


def r_uint(bits: int) -> int:
    # Mix of small values, boundary values, and uniform randoms.
    choice = rng.random()
    if choice < 0.2:
        return rng.choice([0, 1, 2 ** bits - 1, 2 ** (bits - 1)])
    if choice < 0.5:
        return rng.randrange(0, min(2 ** bits, 1 << 16))
    return rng.randrange(0, 2 ** bits)


def r_bool() -> bool:
    return rng.random() < 0.5


def r_bytes(n: int) -> bytes:
    return rng.randbytes(n)


def r_bits(n: int) -> list[bool]:
    return [r_bool() for _ in range(n)]


def r_len(limit: int) -> int:
    choice = rng.random()
    if choice < 0.25:
        return rng.choice([0, limit])
    return rng.randrange(0, limit + 1)


def gen_square() -> PCSquare:
    return PCSquare(side=Uint16(r_uint(16)), color=Uint8(r_uint(8)))


def gen_circle() -> PCCircle:
    return PCCircle(radius=Uint16(r_uint(16)), color=Uint8(r_uint(8)))


def gen_shape() -> SampleShape:
    sel = rng.choice([1, 2, 127])
    data = gen_circle() if sel == 2 else gen_square()
    return SampleShape(selector=Uint8(sel), data=data)


def gen_uint16list4() -> SampleUint16List4:
    return SampleUint16List4(data=[Uint16(r_uint(16)) for _ in range(r_len(4))])


def gen_u64plist() -> SampleUint64ProgressiveList:
    return SampleUint64ProgressiveList(
        data=[Uint64(r_uint(64)) for _ in range(r_len(PROGRESSIVE_CAP))]
    )


def gen_u16plist() -> SampleUint16ProgressiveList:
    return SampleUint16ProgressiveList(
        data=[Uint16(r_uint(16)) for _ in range(r_len(40))]
    )


def gen_pbitlist() -> ProgressiveBitList:
    return ProgressiveBitList(data=r_bits(r_len(PROGRESSIVE_CAP * 8)))


GENERATORS: dict[str, dict[str, Callable[[], Any]]] = {
    "test_basic_types": {
        "Boolean": lambda: Boolean(r_bool()),
        "Uint8": lambda: Uint8(r_uint(8)),
        "Uint16": lambda: Uint16(r_uint(16)),
        "Uint32": lambda: Uint32(r_uint(32)),
        "Uint64": lambda: Uint64(r_uint(64)),
        "Uint128": lambda: Uint128(r_uint(128)),
        "Uint256": lambda: Uint256(r_uint(256)),
        "Bytes4": lambda: Bytes4(r_bytes(4)),
        "Bytes32": lambda: Bytes32(r_bytes(32)),
        "Bytes52": lambda: Bytes52(r_bytes(52)),
        "Bytes64": lambda: Bytes64(r_bytes(64)),
        "ByteList512KiB": lambda: ByteList512KiB(data=r_bytes(r_len(2000))),
        "SampleBitVector8": lambda: SampleBitVector8(data=r_bits(8)),
        "SampleBitVector64": lambda: SampleBitVector64(data=r_bits(64)),
        "SampleBitList16": lambda: SampleBitList16(data=r_bits(r_len(16))),
        "SampleUint16Vector3": lambda: SampleUint16Vector3(
            data=[Uint16(r_uint(16)) for _ in range(3)]
        ),
        "SampleUint64Vector4": lambda: SampleUint64Vector4(
            data=[Uint64(r_uint(64)) for _ in range(4)]
        ),
        "SampleUint32List16": lambda: SampleUint32List16(
            data=[Uint32(r_uint(32)) for _ in range(r_len(16))]
        ),
        "SampleBytes32List8": lambda: SampleBytes32List8(
            data=[Bytes32(r_bytes(32)) for _ in range(r_len(8))]
        ),
    },
    "test_merkleization_boundaries": {
        "BoundaryBitVector1": lambda: BoundaryBitVector1(data=r_bits(1)),
        "BoundaryBitVector7": lambda: BoundaryBitVector7(data=r_bits(7)),
        "BoundaryBitVector9": lambda: BoundaryBitVector9(data=r_bits(9)),
        "BoundaryBitVector255": lambda: BoundaryBitVector255(data=r_bits(255)),
        "BoundaryBitVector256": lambda: BoundaryBitVector256(data=r_bits(256)),
        "BoundaryBitVector257": lambda: BoundaryBitVector257(data=r_bits(257)),
        "BoundaryBitList256": lambda: BoundaryBitList256(data=r_bits(r_len(256))),
        "BoundaryUint64List32": lambda: BoundaryUint64List32(
            data=[Uint64(r_uint(64)) for _ in range(r_len(32))]
        ),
    },
    "test_progressive_types": {
        "SampleUint64ProgressiveList": gen_u64plist,
        "SampleBytes32ProgressiveList": lambda: SampleBytes32ProgressiveList(
            data=[Bytes32(r_bytes(32)) for _ in range(r_len(PROGRESSIVE_CAP))]
        ),
        "SampleUint16ProgressiveList": gen_u16plist,
        "SampleNestedProgressiveList": lambda: SampleNestedProgressiveList(
            data=[gen_u16plist() for _ in range(r_len(20))]
        ),
        "ProgressiveBitList": gen_pbitlist,
        "SampleContainerWithProgressiveList": lambda: SampleContainerWithProgressiveList(
            a=Uint16(r_uint(16)), b=gen_u64plist(), c=Uint8(r_uint(8))
        ),
    },
    "test_progressive_containers": {
        "SampleSquare": gen_square,
        "SampleCircle": gen_circle,
        "SampleOneField": lambda: SampleOneField(a=Uint16(r_uint(16))),
        "SampleLeadingGaps": lambda: SampleLeadingGaps(c=Uint32(r_uint(32))),
        "SampleMultipleGaps": lambda: SampleMultipleGaps(
            a=Uint8(r_uint(8)), b=Uint16(r_uint(16)), c=Uint32(r_uint(32))
        ),
        "SampleWidestLayout": lambda: SampleWidestLayout(tail=Uint8(r_uint(8))),
        "SampleLevelBoundary": lambda: SampleLevelBoundary(
            first=Uint16(r_uint(16)), last=Uint8(r_uint(8))
        ),
        "SampleBoundedListField": lambda: SampleBoundedListField(
            head=Uint64(r_uint(64)), body=gen_uint16list4()
        ),
        "SampleProgressiveFields": lambda: SampleProgressiveFields(
            head=Uint64(r_uint(64)), numbers=gen_u64plist(), flags=gen_pbitlist()
        ),
        "SampleInnerShape": lambda: SampleInnerShape(
            x=Uint16(r_uint(16)), y=Uint8(r_uint(8))
        ),
        "SampleOuterShape": lambda: SampleOuterShape(
            head=Uint8(r_uint(8)),
            inner=SampleInnerShape(x=Uint16(r_uint(16)), y=Uint8(r_uint(8))),
        ),
        "SampleSquareProgressiveList": lambda: PCSquareProgressiveList(
            data=[gen_square() for _ in range(r_len(40))]
        ),
        "SampleShapeContainer": lambda: PCShapeContainer(
            tag=Uint8(r_uint(8)), shape=gen_square()
        ),
        "SampleUint64ProgressiveList": gen_u64plist,
    },
    "test_compatible_unions": {
        "SampleSquare": gen_square,
        "SampleCircle": gen_circle,
        "SampleShape": gen_shape,
        "SampleNumbers": lambda: SampleNumbers(
            selector=Uint8(rng.choice([1, 2])), data=gen_uint16list4()
        ),
        "SampleEmptyProne": lambda: (
            lambda sel: SampleEmptyProne(
                selector=Uint8(sel),
                data=(CUSquareProgressiveList if sel == 1 else CUCircleProgressiveList)(
                    data=[
                        (gen_square() if sel == 1 else gen_circle())
                        for _ in range(r_len(20))
                    ]
                ),
            )
        )(rng.choice([1, 2])),
        "SampleSquareOnly": lambda: SampleSquareOnly(
            selector=Uint8(5), data=gen_square()
        ),
        "SampleNestedShape": lambda: (
            lambda sel: SampleNestedShape(
                selector=Uint8(sel),
                data=gen_shape()
                if sel == 1
                else SampleSquareOnly(selector=Uint8(5), data=gen_square()),
            )
        )(rng.choice([1, 2])),
        "SampleShapeContainer": lambda: CUShapeContainer(
            tag=Uint64(r_uint(64)), body=gen_shape()
        ),
        "SampleShapeProgressiveContainer": lambda: CUShapeProgressiveContainer(
            tag=Uint64(r_uint(64)), body=gen_shape()
        ),
        "SampleShapeProgressiveList": lambda: CUShapeProgressiveList(
            data=[gen_shape() for _ in range(r_len(20))]
        ),
    },
    "test_decode_failure_smoke": {
        "SmokeBitList8": lambda: SmokeBitList8(data=r_bits(r_len(8))),
    },
}


# ---------------------------------------------------------------------------
# Fixture emission.
# ---------------------------------------------------------------------------


def value_to_json(v: Any) -> Any:
    if isinstance(v, SSZModel):
        return v.model_dump(mode="json")
    if isinstance(v, Boolean):
        return bool(v)
    if isinstance(v, bytes):
        return "0x" + v.hex()
    if isinstance(v, int):
        return str(v)
    return str(v)


def mutate(data: bytes) -> bytes:
    out = bytearray(data)
    kind = rng.randrange(4)
    if kind == 0 and out:  # flip bytes
        for _ in range(rng.randrange(1, 5)):
            pos = rng.randrange(len(out))
            out[pos] ^= rng.randrange(1, 256)
    elif kind == 1 and out:  # truncate
        out = out[: rng.randrange(len(out))]
    elif kind == 2:  # extend
        out += rng.randbytes(rng.randrange(1, 9))
    else:  # splice
        pos = rng.randrange(len(out) + 1)
        out[pos:pos] = rng.randbytes(rng.randrange(1, 5))
    return bytes(out)


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", required=True)
    ap.add_argument("--seed", type=int, default=873767)
    ap.add_argument("--per-type", type=int, default=10)
    ap.add_argument("--mutations", type=int, default=25)
    args = ap.parse_args()

    global rng
    rng = random.Random(args.seed)

    out_root = Path(args.out)
    stats = {"positive": 0, "negative": 0, "mutation_accepted": 0}

    for dirname, gens in GENERATORS.items():
        outdir = out_root / dirname
        outdir.mkdir(parents=True, exist_ok=True)
        for type_name, gen in gens.items():
            vectors: dict[str, Any] = {}
            encodings: list[bytes] = []

            for i in range(args.per_type):
                value = gen()
                enc = value.encode_bytes()
                encodings.append(enc)
                root = hash_tree_root(value)
                # Sanity: the spec's own decode must roundtrip.
                assert value.decode_bytes(enc) == value
                vectors[f"corpus::{type_name}_random_{i}"] = {
                    "typeName": type_name,
                    "value": value_to_json(value),
                    "serialized": "0x" + enc.hex(),
                    "root": "0x" + root.hex(),
                }
                stats["positive"] += 1

            cls = type(gen())
            for i in range(args.mutations):
                base = rng.choice(encodings)
                mut = mutate(base)
                try:
                    decoded = cls.decode_bytes(mut)
                except Exception:
                    vectors[f"corpus::{type_name}_mut_{i}"] = {
                        "typeName": type_name,
                        "rejectionReason": "DECODE_ERROR",
                        "value": value_to_json(gen()),
                        "rawBytes": "0x" + mut.hex(),
                        "serialized": "0x" + mut.hex(),
                        "root": "",
                    }
                    stats["negative"] += 1
                    continue
                # Spec accepted the mutation: canonical strictness means it
                # must re-encode identically.
                re_enc = decoded.encode_bytes()
                assert re_enc == mut, (
                    f"SPEC canonicity violation? {type_name}: decode(0x{mut.hex()}) "
                    f"re-encodes to 0x{re_enc.hex()}"
                )
                root = hash_tree_root(decoded)
                vectors[f"corpus::{type_name}_mut_{i}"] = {
                    "typeName": type_name,
                    "value": value_to_json(decoded),
                    "serialized": "0x" + mut.hex(),
                    "root": "0x" + root.hex(),
                }
                stats["mutation_accepted"] += 1

            with open(outdir / f"corpus_{type_name}.json", "w") as f:
                json.dump(vectors, f, indent=1)

    print(f"corpus written to {out_root}: {stats}")


if __name__ == "__main__":
    main()
