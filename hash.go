package growthbook

import (
	"fmt"
	"hash/fnv"

	"github.com/growthbook/growthbook-golang/internal/value"
)

// Main hash function. Default version is 1.
func hash(seed string, attrValue value.Value, version int) *float64 {
	attrStr := attrValue.String()
	switch version {
	case 2:
		v := float64(hashFnv32a(fmt.Sprint(hashFnv32a(seed+attrStr)))%10000) / 10000
		return &v
	case 0, 1:
		v := float64(hashFnv32a(attrStr+seed)%1000) / 1000
		return &v
	default:
		return nil
	}
}

// Simple wrapper around Go standard library FNV32a hash function.
func hashFnv32a(s string) uint32 {
	hash := fnv.New32a()
	hash.Write([]byte(s))
	return hash.Sum32()
}
