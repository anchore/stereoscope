package file

import "sort"

var nextID = 0 // note: this is governed by the reference constructor

// ID is used for file tree manipulation to uniquely identify tree nodes.
type ID uint64

type IDs []ID

func (ids IDs) Len() int {
	return len(ids)
}

func (ids IDs) Less(i, j int) bool {
	return ids[i] < ids[j]
}

func (ids IDs) Swap(i, j int) {
	ids[i], ids[j] = ids[j], ids[i]
}

type IDSet map[ID]struct{}

func NewIDSet() IDSet {
	return make(IDSet)
}

func (s IDSet) Size() int {
	return len(s)
}

func (s IDSet) Merge(other IDSet) {
	for i := range other.Enumerate() {
		s.Add(i)
	}
}

func (s IDSet) Add(ids ...ID) {
	for _, i := range ids {
		s[i] = struct{}{}
	}
}

func (s IDSet) Remove(ids ...ID) {
	for _, i := range ids {
		delete(s, i)
	}
}

func (s IDSet) Contains(i ID) bool {
	_, ok := s[i]
	return ok
}

func (s IDSet) Clear() {
	// TODO: replace this with the new 'clear' keyword when it's available in go 1.20 or 1.21
	for i := range s {
		delete(s, i)
	}
}

func (s IDSet) List() []ID {
	ret := make([]ID, 0, len(s))
	for i := range s {
		ret = append(ret, i)
	}

	sort.Sort(IDs(ret))

	return ret
}

func (s IDSet) Enumerate() <-chan ID {
	ret := make(chan ID)
	go func() {
		defer close(ret)
		for i := range s {
			ret <- i
		}
	}()
	return ret
}

func (s IDSet) ContainsAny(ids ...ID) bool {
	for _, i := range ids {
		_, ok := s[i]
		if ok {
			return true
		}
	}
	return false
}
