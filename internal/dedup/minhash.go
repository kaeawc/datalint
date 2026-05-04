// Package dedup provides MinHash signatures for estimating Jaccard
// similarity between two texts. Used by leakage rules to detect
// near-duplicate prompts across train/eval splits.
package dedup

import (
	"hash/fnv"
	"math/rand"
	"strings"
)

// NumHashes is the signature length. Variance of the Jaccard estimate
// is roughly 1/sqrt(N) — 128 gives ±0.09 standard error, plenty for
// thresholds in the 0.7–0.95 range. Increase for tighter estimates.
const NumHashes = 128

// shingleSize is the n-gram width over whitespace-separated tokens.
// 3 is a balance: small enough to find overlap between short prompts
// that differ in punctuation/contractions, large enough to dilute
// trivial 1- or 2-token matches.
const shingleSize = 3

// prime is a Mersenne prime that fits comfortably below uint64 max,
// so (a*x + b) % prime stays in range without overflow on 64-bit
// arithmetic.
const prime uint64 = (1 << 61) - 1

// MinHash carries the random hash-family parameters. Construct once,
// reuse across many Signature calls within a run; fixing the seed
// makes the signatures comparable across calls and runs.
type MinHash struct {
	a, b []uint64
}

// New returns a MinHash with NumHashes random pairs derived from seed.
// The hash family parameters need to be deterministic and reproducible
// across runs (signatures must compare across invocations) — math/rand
// is the right choice here; crypto/rand would defeat the purpose.
func New(seed int64) *MinHash {
	//nolint:gosec // G404: deterministic PRNG is required, not crypto strength
	rng := rand.New(rand.NewSource(seed))
	m := &MinHash{
		a: make([]uint64, NumHashes),
		b: make([]uint64, NumHashes),
	}
	for i := 0; i < NumHashes; i++ {
		// a must be in [1, prime-1]; b in [0, prime-1].
		m.a[i] = rng.Uint64()%(prime-1) + 1
		m.b[i] = rng.Uint64() % prime
	}
	return m
}

// Signature returns the NumHashes-element MinHash signature for text.
// Returns nil for text too short to shingle (fewer than shingleSize
// tokens after whitespace splitting). nil signatures compare with 0
// similarity to anything, including each other.
func (m *MinHash) Signature(text string) []uint64 {
	shingles := wordShingles(text, shingleSize)
	if len(shingles) == 0 {
		return nil
	}
	sig := make([]uint64, NumHashes)
	for i := range sig {
		sig[i] = ^uint64(0)
	}
	for _, sh := range shingles {
		x := hash64(sh)
		for i := 0; i < NumHashes; i++ {
			v := (m.a[i]*x + m.b[i]) % prime
			if v < sig[i] {
				sig[i] = v
			}
		}
	}
	return sig
}

// Similarity estimates Jaccard similarity from two signatures by
// counting matching positions. Returns 0 when either signature is
// nil or the lengths differ — defensive choices that avoid spurious
// matches between empty signatures.
func Similarity(a, b []uint64) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}
	matches := 0
	for i := range a {
		if a[i] == b[i] {
			matches++
		}
	}
	return float64(matches) / float64(len(a))
}

// wordShingles splits text into k-grams of whitespace-separated
// tokens, lowercased.
func wordShingles(text string, k int) []string {
	fields := strings.Fields(strings.ToLower(text))
	if len(fields) < k {
		return nil
	}
	out := make([]string, 0, len(fields)-k+1)
	for i := 0; i <= len(fields)-k; i++ {
		out = append(out, strings.Join(fields[i:i+k], " "))
	}
	return out
}

func hash64(s string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return h.Sum64()
}
