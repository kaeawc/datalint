package dedup_test

import (
	"testing"

	"github.com/kaeawc/datalint/internal/dedup"
)

func TestSignature_IdenticalTextsSimilarityIs1(t *testing.T) {
	mh := dedup.New(0)
	text := "What is the capital of France today and historically?"
	a := mh.Signature(text)
	b := mh.Signature(text)
	got := dedup.Similarity(a, b)
	if got != 1.0 {
		t.Errorf("identical texts similarity = %f, want 1.0", got)
	}
}

func TestSignature_DisjointTextsSimilarityNearZero(t *testing.T) {
	mh := dedup.New(0)
	a := mh.Signature("the quick brown fox jumps over the lazy dog every morning")
	b := mh.Signature("supercalifragilistic expialidocious entirely unrelated content here")
	got := dedup.Similarity(a, b)
	if got > 0.10 {
		t.Errorf("disjoint similarity = %f, want < 0.10", got)
	}
}

func TestSignature_NearDupSimilarityHigh(t *testing.T) {
	// Test data crafted so the train text's shingles are a proper
	// subset of the eval text's shingles — Jaccard ≈ 8/11 ≈ 0.73.
	mh := dedup.New(0)
	short := mh.Signature("What is the capital of France today and historically?")
	long := mh.Signature("What is the capital of France today and historically? Please answer briefly.")
	got := dedup.Similarity(short, long)
	if got < 0.55 || got > 0.90 {
		// 128-hash MinHash has ~±0.09 std-err around true Jaccard.
		// Keeping this band wide to avoid flakiness.
		t.Errorf("near-dup similarity = %f, want in [0.55, 0.90]", got)
	}
}

func TestSignature_TooShortReturnsNil(t *testing.T) {
	mh := dedup.New(0)
	if got := mh.Signature("a"); got != nil {
		t.Errorf("1-token signature = %v, want nil", got)
	}
	if got := mh.Signature("a b"); got != nil {
		t.Errorf("2-token signature = %v, want nil (k=3)", got)
	}
}

func TestSimilarity_NilSignatures(t *testing.T) {
	mh := dedup.New(0)
	a := mh.Signature("the quick brown fox")
	if got := dedup.Similarity(nil, a); got != 0 {
		t.Errorf("nil vs nonempty = %f, want 0", got)
	}
	if got := dedup.Similarity(a, nil); got != 0 {
		t.Errorf("nonempty vs nil = %f, want 0", got)
	}
	if got := dedup.Similarity(nil, nil); got != 0 {
		t.Errorf("nil vs nil = %f, want 0", got)
	}
}

func TestSignature_DeterministicAcrossInstances(t *testing.T) {
	a := dedup.New(42).Signature("the quick brown fox jumps over")
	b := dedup.New(42).Signature("the quick brown fox jumps over")
	if dedup.Similarity(a, b) != 1.0 {
		t.Error("same seed should produce identical signatures")
	}
}

func TestSignature_DifferentSeedsDifferentSignatures(t *testing.T) {
	a := dedup.New(1).Signature("the quick brown fox jumps over")
	b := dedup.New(2).Signature("the quick brown fox jumps over")
	if dedup.Similarity(a, b) > 0.10 {
		t.Error("different seeds should produce nearly disjoint signatures for the same text")
	}
}
