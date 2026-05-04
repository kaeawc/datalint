package dedup

import (
	"encoding/binary"
	"hash/fnv"
)

// LSH indexes MinHash signatures into bucket bands so similar
// signatures collide on at least one band with high probability.
// Candidates returns the dedup'd index list of signatures that
// share a bucket — callers verify with the full Similarity check.
//
// Defaults: 32 bands of 4 rows each (using NumHashes=128). The
// per-band collision probability for two signatures with Jaccard s
// is s^rows; the chance they collide on at least one band is
// 1 - (1 - s^rows)^bands. With rows=4 and bands=32, the inflection
// is around s ≈ 0.42 — pairs above are almost-certainly retrieved,
// pairs below are mostly skipped. Aggressive bucketing is safe
// because the caller verifies with full similarity.
type LSH struct {
	bands   int
	rows    int
	buckets []map[uint64][]int
}

// DefaultBands and DefaultRows are calibrated for NumHashes=128.
const (
	DefaultBands = 32
	DefaultRows  = 4
)

// NewLSH constructs an empty index with the given band layout.
// bands*rows must not exceed NumHashes.
func NewLSH(bands, rows int) *LSH {
	if bands*rows > NumHashes {
		bands = DefaultBands
		rows = DefaultRows
	}
	buckets := make([]map[uint64][]int, bands)
	for i := range buckets {
		buckets[i] = map[uint64][]int{}
	}
	return &LSH{bands: bands, rows: rows, buckets: buckets}
}

// Add registers entry index idx for the given signature. Signatures
// shorter than bands*rows are silently skipped — the MinHash Signature
// returns nil for too-short text and the rule already guards against
// nil signatures, so this branch is defensive.
func (l *LSH) Add(idx int, sig []uint64) {
	if len(sig) < l.bands*l.rows {
		return
	}
	for b := 0; b < l.bands; b++ {
		h := bandHash(sig, b, l.rows)
		l.buckets[b][h] = append(l.buckets[b][h], idx)
	}
}

// Candidates returns a deduplicated list of indices that share at
// least one band-bucket with sig. Order is band-major then
// insertion-order within band.
func (l *LSH) Candidates(sig []uint64) []int {
	if len(sig) < l.bands*l.rows {
		return nil
	}
	seen := map[int]bool{}
	var out []int
	for b := 0; b < l.bands; b++ {
		h := bandHash(sig, b, l.rows)
		for _, idx := range l.buckets[b][h] {
			if seen[idx] {
				continue
			}
			seen[idx] = true
			out = append(out, idx)
		}
	}
	return out
}

func bandHash(sig []uint64, band, rows int) uint64 {
	start := band * rows
	h := fnv.New64a()
	var buf [8]byte
	for i := 0; i < rows; i++ {
		binary.LittleEndian.PutUint64(buf[:], sig[start+i])
		_, _ = h.Write(buf[:])
	}
	return h.Sum64()
}
