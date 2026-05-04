package dedup_test

import (
	"sort"
	"testing"

	"github.com/kaeawc/datalint/internal/dedup"
)

func TestLSH_EmptyIndex(t *testing.T) {
	lsh := dedup.NewLSH(dedup.DefaultBands, dedup.DefaultRows)
	mh := dedup.New(0)
	if cand := lsh.Candidates(mh.Signature("anything goes here")); len(cand) != 0 {
		t.Errorf("empty index returned %v, want []", cand)
	}
}

func TestLSH_IdenticalSignaturesAlwaysCollide(t *testing.T) {
	lsh := dedup.NewLSH(dedup.DefaultBands, dedup.DefaultRows)
	mh := dedup.New(0)
	sig := mh.Signature("the quick brown fox jumps over the lazy dog")

	lsh.Add(0, sig)
	lsh.Add(1, sig)
	lsh.Add(2, sig)

	got := lsh.Candidates(sig)
	sort.Ints(got)
	if len(got) != 3 || got[0] != 0 || got[1] != 1 || got[2] != 2 {
		t.Errorf("identical-sig candidates = %v, want [0 1 2]", got)
	}
}

func TestLSH_DisjointSignaturesDoNotCollide(t *testing.T) {
	lsh := dedup.NewLSH(dedup.DefaultBands, dedup.DefaultRows)
	mh := dedup.New(0)
	a := mh.Signature("the quick brown fox jumps over the lazy dog every morning")
	b := mh.Signature("supercalifragilistic expialidocious entirely unrelated content here")

	lsh.Add(0, a)

	if cand := lsh.Candidates(b); len(cand) != 0 {
		t.Errorf("disjoint-sig candidates = %v, want []", cand)
	}
}

func TestLSH_NearDupRetrieves(t *testing.T) {
	// Same fixture pair the MinHash test calibrated to ~0.55-0.90
	// similarity. With rows=4 the band-collision probability is
	// effectively 1 in that range — at least one of the 32 bands
	// must match.
	lsh := dedup.NewLSH(dedup.DefaultBands, dedup.DefaultRows)
	mh := dedup.New(0)
	short := mh.Signature("What is the capital of France today and historically?")
	long := mh.Signature("What is the capital of France today and historically? Please answer briefly.")

	lsh.Add(0, short)
	cand := lsh.Candidates(long)
	if len(cand) == 0 || cand[0] != 0 {
		t.Errorf("near-dup not retrieved; candidates = %v", cand)
	}
}

func TestLSH_TooShortSignatureSkipped(t *testing.T) {
	lsh := dedup.NewLSH(dedup.DefaultBands, dedup.DefaultRows)
	short := []uint64{1, 2, 3} // shorter than bands*rows

	lsh.Add(0, short)
	if cand := lsh.Candidates(short); len(cand) != 0 {
		t.Errorf("short-sig query returned %v, want []", cand)
	}
}

func TestLSH_OversizedConfigFallsBackToDefaults(t *testing.T) {
	// bands*rows > NumHashes is unworkable; constructor falls back
	// to defaults silently rather than panicking.
	lsh := dedup.NewLSH(1000, 1000)
	mh := dedup.New(0)
	sig := mh.Signature("the quick brown fox jumps")
	lsh.Add(0, sig)
	if cand := lsh.Candidates(sig); len(cand) != 1 || cand[0] != 0 {
		t.Errorf("fallback layout failed; candidates = %v", cand)
	}
}
