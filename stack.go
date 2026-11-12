package growthbook

import (
	"slices"
)

// stackInlineSize is how many entries a stack holds before it allocates.
// Prerequisite chains are rarely deeper than this.
const stackInlineSize = 4

// stack is a LIFO stack whose first entries live in an inline array, so a
// shallow stack doesn't allocate. It never points into its own array, which
// would force its owner onto the heap.
type stack[T comparable] struct {
	inline [stackInlineSize]T
	n      int
	// overflow holds every entry once the stack outgrows inline
	overflow []T
}

func (s *stack[T]) push(v T) {
	if s.overflow == nil && s.n < stackInlineSize {
		s.inline[s.n] = v
		s.n++
		return
	}
	if s.overflow == nil {
		s.overflow = append(make([]T, 0, 2*stackInlineSize), s.inline[:s.n]...)
	}
	s.overflow = append(s.overflow, v)
	s.n++
}

func (s *stack[T]) pop() (T, bool) {
	var z T
	if s.n == 0 {
		return z, false
	}
	s.n--
	if s.overflow != nil {
		res := s.overflow[s.n]
		s.overflow = s.overflow[:s.n]
		return res, true
	}
	res := s.inline[s.n]
	s.inline[s.n] = z
	return res, true
}

func (s *stack[T]) has(v T) bool {
	if s.overflow != nil {
		return slices.Contains(s.overflow, v)
	}
	return slices.Contains(s.inline[:s.n], v)
}
