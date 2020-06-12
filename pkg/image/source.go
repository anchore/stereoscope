package image

import (
	"strings"
)

const (
	UnknownSource Source = iota
	DockerTarballSource
	DockerDaemonSource
)

var sourceStr = [...]string{
	"UnknownSource",
	"DockerTarball",
	"DockerDaemon",
}

var AllSources = []Source{
	DockerTarballSource,
	DockerDaemonSource,
}

type Source uint8

func ParseImageSpec(imageSpec string) (Source, string) {
	candidates := strings.Split(imageSpec, "://")

	var source Source
	switch len(candidates) {
	case 1:
		source = DockerDaemonSource
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
		return DockerTarballSource
	case "docker", "docker-daemon", "docker-engine":
		return DockerDaemonSource
	case "podman":
		// TODO: implement
		return UnknownSource
	}
	return UnknownSource
}

func (t Source) String() string {
	return sourceStr[t]
}
