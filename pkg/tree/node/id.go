package node

type ID string

type Set map[ID]struct{}

func NewIDSet() Set {
	return make(Set)
}

func (s Set) Add(i ID) {
	s[i] = struct{}{}
}

func (s Set) Remove(i ID) {
	delete(s, i)
}

func (s Set) Contains(i ID) bool {
	_, ok := s[i]
	return ok
}
