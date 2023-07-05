package growthbook

import (
	"fmt"
	"hash/fnv"
)

// Convert integer or string hash values to strings.
func convertHashValue(vin interface{}) (string, bool) {
	hashString, stringOK := vin.(string)
	if stringOK {
		if hashString == "" {
			logInfo("Skip because of empty hash attribute")
			return "", false
		}
		return hashString, true
	}
	hashInt, intOK := vin.(int)
	if intOK {
		return fmt.Sprint(hashInt), true
	}
	hashFloat, floatOK := vin.(float64)
	if floatOK {
		return fmt.Sprint(int(hashFloat)), true
	}
	return "", false
}

// Simple wrapper around Go standard library FNV32a hash function.
func hashFnv32a(s string) uint32 {
	hash := fnv.New32a()
	hash.Write([]byte(s))
	return hash.Sum32()
}

// Main hash function.
func hash(seed string, value string, version int) *float64 {
	switch version {
	case 2:
		v := float64(hashFnv32a(fmt.Sprint(hashFnv32a(seed+value)))%10000) / 10000
		return &v
	case 1:
		v := float64(hashFnv32a(value+seed)%1000) / 1000
		return &v
	default:
		return nil
	}
}
