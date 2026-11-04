package growthbook

import (
	"strconv"
	"unicode/utf16"
)

// Main hash function. Default version is 1.
func hash(seed string, hashValue string, version int) *float64 {
	switch version {
	case 2:
		v := float64(hashFnv32a(strconv.FormatUint(uint64(hashFnv32a(seed+hashValue)), 10))%10000) / 10000
		return &v
	case 0, 1:
		v := float64(hashFnv32a(hashValue+seed)%1000) / 1000
		return &v
	default:
		return nil
	}
}

// hashFnv32a is 32-bit FNV-1a over the string's UTF-16 code units, matching
// the JS SDK, which XORs each charCodeAt value into the hash.
func hashFnv32a(s string) uint32 {
	const (
		offsetBasis uint32 = 0x811c9dc5
		prime       uint32 = 0x01000193
	)
	h := offsetBasis
	// Astral characters are two code units (a surrogate pair) in UTF-16.
	for _, r := range s {
		if r < 0x10000 {
			h = (h ^ uint32(r)) * prime
			continue
		}
		r1, r2 := utf16.EncodeRune(r)
		h = (h ^ uint32(r1)) * prime
		h = (h ^ uint32(r2)) * prime
	}
	return h
}
