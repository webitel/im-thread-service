package set

import "iter"

var (
	sentinel = nothing{}
)

type (
	nothing struct{}

	Set[T comparable] struct {
		items map[T]nothing
	}
)

func New[T comparable](size int) *Set[T] {
	return &Set[T]{
		items: make(map[T]nothing, max(0, size)),
	}
}

func (s *Set[T]) Insert(item T) bool {
	if _, exists := s.items[item]; exists {
		return false
	}

	if s.items == nil {
		s.items = make(map[T]nothing)
	}
	s.items[item] = sentinel

	return true
}

func (s *Set[T]) InsertSlice(items []T) bool {
	var (
		modified bool
	)

	for _, item := range items {
		if s.Insert(item) {
			modified = true
		}
	}

	return modified
}

func (s *Set[T]) Items() iter.Seq[T] {
	return func(yield func(T) bool) {
		for item := range s.items {
			if !yield(item) {
				return
			}
		}
	}
}

func (s *Set[T]) Slice() []T {
	var (
		result = make([]T, 0, len(s.items))
	)

	for item := range s.items {
		result = append(result, item)
	}

	return result
}
