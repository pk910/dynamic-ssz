import SszCert.Hasher.Batch

/-!
# The innermost scope

Every hasher call works on the innermost open scope, whose region is the end
of the buffer. These lemmas locate it and show that rewriting it leaves the
rest of the buffer, and the holes there, untouched.
-/

namespace Ssz.Hasher

variable {C : Type}

/-- Everything below the innermost scope's region. -/
def St.below (s : St C) : List (Cell C) :=
  s.base ++ ((s.stack.drop 1).reverse.map Layer.cells).flatten

theorem St.cells_top (s : St C) (l : Layer C) (ls : List (Layer C)) (h : s.stack = l :: ls) :
    s.cells = s.below ++ l.cells := by
  simp [St.cells, St.below, h]

theorem St.topStart_eq (s : St C) (l : Layer C) (ls : List (Layer C)) (h : s.stack = l :: ls) :
    s.topStart = s.below.length := by
  simp [St.topStart, St.below, h, List.length_flatten, List.map_map, Function.comp_def]

theorem St.setTop_stack (s : St C) (l l' : Layer C) (ls : List (Layer C)) (h : s.stack = l :: ls) :
    (s.setTop l').stack = l' :: ls := by
  simp [St.setTop, h]

theorem St.setTop_below (s : St C) (l l' : Layer C) (ls : List (Layer C)) (h : s.stack = l :: ls) :
    (s.setTop l').below = s.below := by
  simp [St.setTop, St.below, h]

theorem St.setTop_cells (s : St C) (l l' : Layer C) (ls : List (Layer C)) (h : s.stack = l :: ls) :
    (s.setTop l').cells = s.below ++ l'.cells := by
  rw [St.cells_top _ l' ls (St.setTop_stack s l l' ls h), St.setTop_below s l l' ls h]

theorem St.setTop_jobs (s : St C) (l' : Layer C) : (s.setTop l').jobs = s.jobs := by
  unfold St.setTop; split <;> rfl

theorem St.setTop_err (s : St C) (l' : Layer C) : (s.setTop l').err = s.err := by
  unfold St.setTop; split <;> rfl

theorem St.setTop_jobMaxEnd (s : St C) (l' : Layer C) : (s.setTop l').jobMaxEnd = s.jobMaxEnd := by
  unfold St.setTop; split <;> rfl

/-- **Holes stay in place** when the innermost region is replaced by one that
    keeps its first `k` cells and puts no hole after them, provided no job's
    hole reaches into the replaced part. -/
theorem holesOK_setTop (s : St C) (l l' : Layer C) (ls : List (Layer C)) (h : s.stack = l :: ls)
    (hok : HolesOK s) (k : Nat) (hk : k ≤ l.cells.length) (hpre : l'.cells.take k = l.cells.take k)
    (hk' : k ≤ l'.cells.length)
    (hnew : ∀ i (hi : k ≤ i) (hi' : i < l'.cells.length), (l'.cells[i]'hi').isReady = true)
    (hjob : ∀ j ∈ s.jobs, j.dst + j.out.length ≤ s.below.length + k) :
    HolesOK (s.setTop l') := by
  have hc := St.setTop_cells s l l' ls h
  have hc0 := St.cells_top s l ls h
  -- the two buffers agree on their first `below + k` cells
  have hagree : ∀ i, i < s.below.length + k → (s.setTop l').cells[i]? = s.cells[i]? := by
    intro i hi
    rw [hc, hc0]
    by_cases hb : i < s.below.length
    · rw [List.getElem?_append_left hb, List.getElem?_append_left hb]
    · rw [List.getElem?_append_right (by omega), List.getElem?_append_right (by omega)]
      have hik : i - s.below.length < k := by omega
      have e1 : l'.cells[i - s.below.length]? = (l'.cells.take k)[i - s.below.length]? := by
        rw [List.getElem?_take, if_pos hik]
      have e2 : l.cells[i - s.below.length]? = (l.cells.take k)[i - s.below.length]? := by
        rw [List.getElem?_take, if_pos hik]
      rw [e1, e2, hpre]
  refine ⟨?_, ?_, ?_, ?_⟩
  · intro j hj kk hkk
    rw [St.setTop_jobs] at hj
    obtain ⟨idx, hidx⟩ := hok.inplace j hj kk hkk
    have := hjob j hj
    exact ⟨idx, by rw [hagree _ (by omega)]; exact hidx⟩
  · intro p v id idx hp
    rw [St.setTop_jobs]
    by_cases hpk : p < s.below.length + k
    · rw [hagree p hpk] at hp; exact hok.owned p v id idx hp
    · exfalso
      rw [hc, List.getElem?_append_right (by omega)] at hp
      have hlt : p - s.below.length < l'.cells.length := by
        rcases Nat.lt_or_ge (p - s.below.length) l'.cells.length with h1 | h1
        · exact h1
        · rw [List.getElem?_eq_none h1] at hp; cases hp
      have := hnew (p - s.below.length) (by omega) hlt
      rw [List.getElem?_eq_getElem hlt] at hp
      simp only [Option.some.injEq] at hp
      rw [hp] at this; simp [Cell.isReady] at this
  · rw [St.setTop_jobs]; exact hok.distinct
  · intro j hj; rw [St.setTop_jobs] at hj; rw [St.setTop_jobMaxEnd]; exact hok.maxEnd j hj

end Ssz.Hasher
