package value

import "strconv"

// Path resolves object keys, array indexes, and array length, returning Null
// when a segment is missing or cannot be traversed.
func Path(actual Value, path ...string) Value {
	for _, part := range path {
		switch current := actual.(type) {
		case ObjValue:
			actual = current[part]
		case ArrValue:
			if part == "length" {
				actual = Num(len(current))
				continue
			}
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(current) || strconv.Itoa(index) != part {
				return Null()
			}
			actual = current[index]
		default:
			return Null()
		}
		if actual == nil {
			return Null()
		}
	}
	return actual
}
