package file

type PathSet map[Path]struct{}

func NewPathSet() PathSet {
	return make(PathSet)
}

func (s PathSet) Add(i Path) {
	s[i] = struct{}{}
}

func (s PathSet) Remove(i Path) {
	delete(s, i)
}

func (s PathSet) Contains(i Path) bool {
	_, ok := s[i]
	return ok
}
