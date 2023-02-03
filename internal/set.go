package internal

import (
	"golang.org/x/exp/constraints"
	"sort"
)

type orderedComparable interface {
	comparable
	constraints.Ordered
}

type OrderableSet[T orderedComparable] map[T]struct{}

func NewOrderableSet[T orderedComparable](is ...T) OrderableSet[T] {
	s := make(OrderableSet[T])
	s.Add(is...)
	return s
}

func NewStringSet(start ...string) OrderableSet[string] {
	return NewOrderableSet(start...)
}

func (s OrderableSet[T]) Size() int {
	return len(s)
}

func (s OrderableSet[T]) Merge(other OrderableSet[T]) {
	for _, i := range other.List() {
		s.Add(i)
	}
}

func (s OrderableSet[T]) Add(ids ...T) {
	for _, i := range ids {
		s[i] = struct{}{}
	}
}

func (s OrderableSet[T]) Remove(ids ...T) {
	for _, i := range ids {
		delete(s, i)
	}
}

func (s OrderableSet[T]) Contains(i T) bool {
	_, ok := s[i]
	return ok
}

func (s OrderableSet[T]) Clear() {
	// TODO: replace this with the new 'clear' keyword when it's available in go 1.20 or 1.21
	for i := range s {
		delete(s, i)
	}
}

func (s OrderableSet[T]) List() []T {
	ret := make([]T, 0, len(s))
	for i := range s {
		ret = append(ret, i)
	}
	return ret
}

func (s OrderableSet[T]) Sorted() []T {
	ids := s.List()

	sort.Slice(ids, func(i, j int) bool {
		return ids[i] < ids[j]
	})

	return ids
}

func (s OrderableSet[T]) ContainsAny(ids ...T) bool {
	for _, i := range ids {
		_, ok := s[i]
		if ok {
			return true
		}
	}
	return false
}
