package growthbook

import (
	"slices"
)

type stack[T comparable] struct {
	stack []T
}

func (s *stack[T]) push(v T) {
	s.stack = append(s.stack, v)
}

func (s *stack[T]) pop() (T, bool) {
	if len(s.stack) == 0 {
		var z T
		return z, false
	}
	res := s.stack[len(s.stack)-1]
	s.stack = s.stack[:len(s.stack)-1]
	return res, true
}

func (s *stack[T]) has(v T) bool {
	return slices.Contains(s.stack, v)
}
