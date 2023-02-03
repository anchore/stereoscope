package node

import "github.com/anchore/stereoscope/internal"

type ID string

type IDSet = internal.OrderableSet[ID]

func NewIDSet(ids ...ID) IDSet {
	return internal.NewOrderableSet(ids...)
}
