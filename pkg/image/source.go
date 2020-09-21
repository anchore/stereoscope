package image

import (
	"strings"
)

const (
	UnknownSource Source = iota
	DockerTarballSource
	DockerDaemonSource
	OciDirectorySource
	OciTarballSource
	ContainersStorageSource
)

var sourceStr = [...]string{
	"UnknownSource",
	"DockerTarball",
	"DockerDaemon",
	"OciDirectory",
	"OciTarball",
	"ContainersStorage",
}

var AllSources = []Source{
	DockerTarballSource,
	DockerDaemonSource,
	OciDirectorySource,
	OciTarballSource,
	ContainersStorageSource,
}

// Source is a concrete a selection of valid concrete image providers.
type Source uint8

// ParseImageSpec takes a user string and determines the image source (e.g. the docker daemon, a tar file, etc.) and
// image identifier for future fetching (e.g. "wagoodman/dive:latest" or "./image.tar").
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

// ParseSource attempts to resolve a concrete image source selection from a user string (e.g. "docker://", "tar://", "podman://", etc.)
func ParseSource(source string) Source {
	source = strings.ToLower(source)
	switch source {
	case "tarball", "tar", "archive", "docker-archive", "docker-tar", "docker-tarball":
		return DockerTarballSource
	case "docker", "docker-daemon", "docker-engine":
		return DockerDaemonSource
	case "oci", "oci-directory", "oci-dir":
		return OciDirectorySource
	case "oci-tarball", "oci-tar", "oci-archive":
		return OciTarballSource
	case "podman", "containers-storage", "container-storage", "cs":
		return ContainersStorageSource
	}
	return UnknownSource
}

// String returns a convenient display string for the source.
func (t Source) String() string {
	return sourceStr[t]
}
