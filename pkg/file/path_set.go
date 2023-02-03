package file

import "github.com/anchore/stereoscope/internal"

type PathSet = internal.OrderableSet[Path]

func NewPathSet(paths ...Path) PathSet {
	return internal.NewOrderableSet(paths...)
}
