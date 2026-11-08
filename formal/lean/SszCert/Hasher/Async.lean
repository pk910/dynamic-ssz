import SszCert.Hasher.Binary

/-!
# Background jobs

A job snapshots its input when it is enqueued and leaves a hole carrying its
result; `capJob`/`capCompact` (one round of `precollapseCapRun`) replace a
cap-wide run of leaves by one such hole. This file proves the round keeps
the scope's cover and the hole invariant.
-/

namespace Ssz.Hasher

variable {C : Type}

/-- **Adding a job** whose hole sits right after the first `k` cells of the
    innermost region (everything after it ready, every older job below it). -/
theorem holesOK_addJob (s : St C) (l l' : Layer C) (ls : List (Layer C)) (h : s.stack = l :: ls)
    (hok : HolesOK s) (j : Job C) (hfresh : ∀ j' ∈ s.jobs, j'.id ≠ j.id)
    (k : Nat) (hk : k ≤ l.cells.length) (hpre : l'.cells.take k = l.cells.take k)
    (hdst : j.dst = s.below.length + k)
    (hhole : ∀ i (hi : i < j.out.length), ∃ idx, l'.cells[k + i]? = some (.hole (j.out[i]'hi) j.id idx))
    (hlen : k + j.out.length ≤ l'.cells.length)
    (hnew : ∀ i (hi : k + j.out.length ≤ i) (hi' : i < l'.cells.length), (l'.cells[i]'hi').isReady = true)
    (hjob : ∀ j' ∈ s.jobs, j'.dst + j'.out.length ≤ s.below.length + k) (m : Nat) :
    HolesOK (St.setTop { s with jobs := s.jobs ++ [j], nextJob := m, jobMaxEnd := max s.jobMaxEnd (j.dst + j.out.length) } l') := by
  let t : St C := { s with jobs := s.jobs ++ [j], nextJob := m, jobMaxEnd := max s.jobMaxEnd (j.dst + j.out.length) }
  have ht : t.stack = l :: ls := h
  have htb : t.below = s.below := rfl
  have hc := St.setTop_cells t l l' ls ht
  rw [htb] at hc
  have hc0 := St.cells_top s l ls h
  have hagree : ∀ i, i < s.below.length + k → (t.setTop l').cells[i]? = s.cells[i]? := by
    intro i hi
    rw [hc, hc0]
    by_cases hb : i < s.below.length
    · rw [List.getElem?_append_left hb, List.getElem?_append_left hb]
    · rw [List.getElem?_append_right (by omega), List.getElem?_append_right (by omega)]
      have hik : i - s.below.length < k := by omega
      rw [show l'.cells[i - s.below.length]? = (l'.cells.take k)[i - s.below.length]? by
            rw [List.getElem?_take, if_pos hik],
        show l.cells[i - s.below.length]? = (l.cells.take k)[i - s.below.length]? by
            rw [List.getElem?_take, if_pos hik], hpre]
  have hjobs : (t.setTop l').jobs = s.jobs ++ [j] := by rw [St.setTop_jobs]
  refine ⟨?_, ?_, ?_, ?_⟩
  · intro j' hj' kk hkk
    rw [hjobs] at hj'
    rcases List.mem_append.mp hj' with hj' | hj'
    · obtain ⟨idx, hidx⟩ := hok.inplace j' hj' kk hkk
      have := hjob j' hj'
      exact ⟨idx, by rw [hagree _ (by omega)]; exact hidx⟩
    · simp at hj'; subst hj'
      obtain ⟨idx, hidx⟩ := hhole kk hkk
      refine ⟨idx, ?_⟩
      rw [hc, hdst, List.getElem?_append_right (by omega)]
      rw [show s.below.length + k + kk - s.below.length = k + kk by omega]; exact hidx
  · intro p v id idx hp
    rw [hjobs]
    by_cases hpk : p < s.below.length + k
    · rw [hagree p hpk] at hp
      obtain ⟨j', hj', hid, h1, h2⟩ := hok.owned p v id idx hp
      exact ⟨j', List.mem_append_left _ hj', hid, h1, h2⟩
    · rw [hc, List.getElem?_append_right (by omega)] at hp
      have hlt : p - s.below.length < l'.cells.length := by
        rcases Nat.lt_or_ge (p - s.below.length) l'.cells.length with h1 | h1
        · exact h1
        · rw [List.getElem?_eq_none h1] at hp; cases hp
      by_cases hin : p - s.below.length < k + j.out.length
      · obtain ⟨idx2, h2⟩ := hhole (p - s.below.length - k) (by omega)
        rw [show k + (p - s.below.length - k) = p - s.below.length by omega, hp] at h2
        simp at h2
        exact ⟨j, by simp, h2.2.1.symm, by omega, by omega⟩
      · have := hnew (p - s.below.length) (by omega) hlt
        rw [List.getElem?_eq_getElem hlt] at hp
        simp only [Option.some.injEq] at hp
        rw [hp] at this; simp [Cell.isReady] at this
  · rw [hjobs, List.map_append, List.nodup_append]
    refine ⟨hok.distinct, by simp, ?_⟩
    intro a ha b hb
    simp at hb; subst hb
    obtain ⟨j', hj', rfl⟩ := List.mem_map.mp ha
    exact hfresh j' hj'
  · intro j' hj'
    rw [hjobs] at hj'
    rw [St.setTop_jobMaxEnd]
    show j'.dst + j'.out.length ≤ max s.jobMaxEnd (j.dst + j.out.length)
    rcases List.mem_append.mp hj' with hj' | hj'
    · have := hok.maxEnd j' hj'; omega
    · simp at hj'; subst hj'; omega

end Ssz.Hasher
