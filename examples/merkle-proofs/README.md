# Merkle Proofs Example

Build Merkle trees from SSZ structures, prove individual fields — including
elements deep inside lists — and verify the proofs standalone.

This is the pattern used to prove validator and withdrawal data out of the
beacon state for on-chain verification: build the tree with
`GetTree` + `Prove`, and submit the resulting sibling-hash chains to a
contract that verifies them against a known state root.

## Run

```bash
go run .
```

## What it shows

1. **`GetTree`** — build the complete Merkle tree once; its root hash equals
   `HashTreeRoot`. Reuse the tree for every proof (rebuilding per proof is
   the expensive mistake).
2. **`tree.Show(depth)`** — inspect the tree layout to discover generalized
   indices.
3. **Generalized-index arithmetic** — walking from the root to a validator
   inside the `Validators` list:

   ```go
   gid := uint64(1)                                // root
   gid = gid*stateChunkCeil + validatorsFieldIndex // container field (8 fields → ceil 8)
   gid *= 2                                        // list data subtree (length mix-in is the right child)
   gid = gid*validatorsLimit + validatorIndex      // element within the limit-sized subtree
   ```

4. **Leaf sanity check** — before trusting a proof, compare its leaf against
   the field's own `HashTreeRoot`. Always do this for proofs that get
   submitted on-chain.
5. **`ProveMulti` / `VerifyMultiproof`** — prove several fields with shared
   sibling hashes; three single proofs would carry 9 hashes here, the
   multiproof carries 3.
6. **Standalone `VerifyProof`** — verification needs only the root and the
   proof. The tree itself can be dropped immediately (a full mainnet state
   tree is the memory hot spot — release it as soon as the proofs exist).

## Notes

- Generalized indices depend on the container layout: with 8 fields the
  container merkleizes into 8 chunks, so field *i* sits at index `8+i`.
  Adding a 9th field would double the chunk ceiling to 16.
- Progressive containers (`ssz-index` tags) produce different tree shapes and
  therefore different indices — use `tree.Show()` to discover them. See
  [docs/merkle-proofs.md](../../docs/merkle-proofs.md).
- For preset-dependent types, pass the chain specs to `NewDynSsz` — list
  limits change the subtree depth and with it every generalized index below.
