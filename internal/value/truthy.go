package value

import "math"

// Truthy reports whether v is truthy under JS coercion rules:
// null, false, 0, NaN and "" are falsy; everything else (including
// empty arrays and objects) is truthy.
func Truthy(v Value) bool {
	switch v.Type() {
	case NullType:
		return false
	case BoolType:
		return bool(v.(BoolValue))
	case NumType:
		n := float64(v.(NumValue))
		return n != 0 && !math.IsNaN(n)
	case StrType:
		return v.(StrValue) != ""
	default:
		return true
	}
}
