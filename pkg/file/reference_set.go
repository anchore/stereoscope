package file

type ReferenceSet map[ID]struct{}

func NewFileReferenceSet() ReferenceSet {
	return make(ReferenceSet)
}

func (s ReferenceSet) Add(ref Reference) {
	s[ref.ID()] = struct{}{}
}

func (s ReferenceSet) Remove(ref Reference) {
	delete(s, ref.ID())
}

func (s ReferenceSet) Contains(ref Reference) bool {
	_, ok := s[ref.ID()]
	return ok
}
