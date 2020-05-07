package image

import (
	"net/url"
	"strings"
)

const (
	UnknownSource Source = iota
	TarballSource
)

var sourceStr = [...]string{
	"UnknownSource",
	"Tarball",
}

type Source uint8

func ParseImageSpec(imageSpec string) (Source, string) {
	u, err := url.Parse(imageSpec)
	if err != nil {
		return UnknownSource, ""
	}
	return ParseSource(u.Scheme), strings.TrimPrefix(imageSpec, u.Scheme+"://")
}

func ParseSource(source string) Source {
	switch source {
	case "tarball":
		return TarballSource
	}
	return UnknownSource
}

func (t Source) String() string {
	return sourceStr[t]
}
