package file

import (
	"github.com/anchore/stereoscope/internal"
)

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

type IDSet = internal.OrderableSet[ID]

func NewIDSet(ids ...ID) IDSet {
	return internal.NewOrderableSet(ids...)
}
