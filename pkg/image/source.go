package image

import (
	"strings"
)

const (
	UnknownSource Source = iota
	TarballSource
	DockerSource
)

var sourceStr = [...]string{
	"UnknownSource",
	"Tarball",
	"Docker",
}

type Source uint8

func ParseImageSpec(imageSpec string) (Source, string) {
	candidates := strings.Split(imageSpec, "://")

	var source Source
	switch len(candidates) {
	case 1:
		source = DockerSource
	case 2:
		source = ParseSource(candidates[0])
	default:
		source = UnknownSource
	}

	if source == UnknownSource {
		return source, ""
	}

	return source, strings.TrimPrefix(imageSpec, candidates[0]+"://")
}

func ParseSource(source string) Source {
	source = strings.ToLower(source)
	switch source {
	case "tarball", "tar", "archive", "docker-archive":
		return TarballSource
	case "docker", "docker-daemon", "docker-engine":
		return DockerSource
	case "podman":
		// TODO: implement
		return UnknownSource
	}
	return UnknownSource
}

func (t Source) String() string {
	return sourceStr[t]
}
