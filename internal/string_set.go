package internal

import (
	"sort"
)

type StringSet map[string]struct{}

func NewStringSet(is ...string) StringSet {
	// TODO: replace with single generic implementation that also incorporates other set implementations
	s := make(StringSet)
	s.Add(is...)
	return s
}

func (s StringSet) Size() int {
	return len(s)
}

func (s StringSet) Merge(other StringSet) {
	for _, i := range other.List() {
		s.Add(i)
	}
}

func (s StringSet) Add(ids ...string) {
	for _, i := range ids {
		s[i] = struct{}{}
	}
}

func (s StringSet) Remove(ids ...string) {
	for _, i := range ids {
		delete(s, i)
	}
}

func (s StringSet) Contains(i string) bool {
	_, ok := s[i]
	return ok
}

func (s StringSet) Clear() {
	clear(s)
}

func (s StringSet) List() []string {
	ret := make([]string, 0, len(s))
	for i := range s {
		ret = append(ret, i)
	}
	return ret
}

func (s StringSet) Sorted() []string {
	ids := s.List()

	sort.Slice(ids, func(i, j int) bool {
		return ids[i] < ids[j]
	})

	return ids
}

func (s StringSet) ContainsAny(ids ...string) bool {
	for _, i := range ids {
		_, ok := s[i]
		if ok {
			return true
		}
	}
	return false
}
