package image

import (
	"io"
	"os"
	"path"
	"strings"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/spf13/afero"
)

const (
	UnknownSource Source = iota
	DockerTarballSource
	DockerDaemonSource
	OciDirectorySource
	OciTarballSource
)

var sourceStr = [...]string{
	"UnknownSource",
	"DockerTarball",
	"DockerDaemon",
	"OciDirectory",
	"OciTarball",
}

var AllSources = []Source{
	DockerTarballSource,
	DockerDaemonSource,
	OciDirectorySource,
	OciTarballSource,
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
		// no source hint has been provided, detect one
		source = DetectSourceFromPath(imageSpec)
		if source == UnknownSource {
			// when all else fails, default to docker daemon
			source = DockerDaemonSource
		}
	case 2:
		// the user has provided the source hint
		source = ParseSource(candidates[0])
	default:
		source = UnknownSource
	}

	if source == UnknownSource {
		return source, ""
	}

	return source, strings.TrimPrefix(imageSpec, candidates[0]+"://")
}

// ParseSource attempts to resolve a concrete image source selection from a user string (e.g. "docker://", "tar://", "podman://", etc.).
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
	case "podman":
		// TODO: implement
		return UnknownSource
	}
	return UnknownSource
}

// DetectSourceFromPath will distinguish between a oci-layout dir, oci-archive, and a docker-archive.
func DetectSourceFromPath(imgPath string) Source {
	return detectSourceFromPath(afero.NewOsFs(), imgPath)
}

// detectSourceFromPath will distinguish between a oci-layout dir, oci-archive, and a docker-archive for a given filesystem.
func detectSourceFromPath(fs afero.Fs, imgPath string) Source {
	pathStat, err := fs.Stat(imgPath)
	if os.IsNotExist(err) {
		return UnknownSource
	} else if err != nil {
		return UnknownSource
	}

	if pathStat.IsDir() {
		//  check for oci-directory
		if _, err := fs.Stat(path.Join(imgPath, "oci-layout")); !os.IsNotExist(err) {
			return OciDirectorySource
		}

		// there are no other directory-based source formats supported
		return UnknownSource
	}

	// assume this is an archive...
	archive, err := fs.Open(imgPath)
	if err != nil {
		return UnknownSource
	}

	for _, pair := range []struct {
		path   string
		source Source
	}{
		{
			"manifest.json",
			DockerTarballSource,
		},
		{
			"oci-layout",
			OciTarballSource,
		},
	} {
		if _, err = archive.Seek(0, io.SeekStart); err != nil {
			return UnknownSource
		}

		_, err = file.ReaderFromTar(archive, pair.path)
		if err == nil {
			return pair.source
		}
	}

	// there are no other archive-based formats supported
	return UnknownSource
}

// String returns a convenient display string for the source.
func (t Source) String() string {
	return sourceStr[t]
}
