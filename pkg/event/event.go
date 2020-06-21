package event

import (
	"github.com/wagoodman/go-partybus"
)

const (
	FetchImage partybus.EventType = "fetch-image-event"
	ReadImage  partybus.EventType = "read-image-event"
	ReadLayer  partybus.EventType = "read-layer-event"
)
