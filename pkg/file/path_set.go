package file

type PathSet map[Path]struct{}

func NewPathSet(is ...Path) PathSet {
	s := make(PathSet)
	s.Add(is...)
	return s
}

func (s PathSet) Size() int {
	return len(s)
}

func (s PathSet) Merge(other PathSet) {
	for _, i := range other.List() {
		s.Add(i)
	}
}

func (s PathSet) Add(ids ...Path) {
	for _, i := range ids {
		s[i] = struct{}{}
	}
}

func (s PathSet) Remove(ids ...Path) {
	for _, i := range ids {
		delete(s, i)
	}
}

func (s PathSet) Contains(i Path) bool {
	_, ok := s[i]
	return ok
}

func (s PathSet) Clear() {
	// TODO: replace this with the new 'clear' keyword when it's available in go 1.20 or 1.21
	for i := range s {
		delete(s, i)
	}
}

func (s PathSet) List() []Path {
	ret := make([]Path, 0, len(s))
	for i := range s {
		ret = append(ret, i)
	}
	return ret
}

func (s PathSet) ContainsAny(ids ...Path) bool {
	for _, i := range ids {
		_, ok := s[i]
		if ok {
			return true
		}
	}
	return false
}
