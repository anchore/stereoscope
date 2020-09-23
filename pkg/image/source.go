package image

import (
	"errors"
	"fmt"
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
func ParseImageSpec(imageSpec string) (Source, string, error) {
	candidates := strings.Split(imageSpec, "://")

	var source Source
	var err error
	switch len(candidates) {
	case 1:
		// no source hint has been provided, detect one
		source, err = DetectSourceFromPath(imageSpec)
		if err != nil {
			return UnknownSource, "", err
		}
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
		return source, "", nil
	}

	return source, strings.TrimPrefix(imageSpec, candidates[0]+"://"), nil
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
func DetectSourceFromPath(imgPath string) (Source, error) {
	return detectSourceFromPath(afero.NewOsFs(), imgPath)
}

// detectSourceFromPath will distinguish between a oci-layout dir, oci-archive, and a docker-archive for a given filesystem.
func detectSourceFromPath(fs afero.Fs, imgPath string) (Source, error) {
	pathStat, err := fs.Stat(imgPath)
	if os.IsNotExist(err) {
		return UnknownSource, nil
	} else if err != nil {
		return UnknownSource, fmt.Errorf("failed to open path=%s: %w", imgPath, err)
	}

	if pathStat.IsDir() {
		//  check for oci-directory
		if _, err := fs.Stat(path.Join(imgPath, "oci-layout")); !os.IsNotExist(err) {
			return OciDirectorySource, nil
		}

		// there are no other directory-based source formats supported
		return UnknownSource, nil
	}

	// assume this is an archive...
	archive, err := fs.Open(imgPath)
	if err != nil {
		return UnknownSource, fmt.Errorf("unable to open archive=%s: %w", imgPath, err)
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
			return UnknownSource, fmt.Errorf("unable to seek archive=%s: %w", imgPath, err)
		}

		var fileErr *file.ErrFileNotFound
		_, err = file.ReaderFromTar(archive, pair.path)
		if err == nil {
			return pair.source, nil
		} else if !errors.As(err, &fileErr) {
			// short-circuit, there is something wrong with the tar reading process
			return UnknownSource, err
		}
	}

	// there are no other archive-based formats supported
	return UnknownSource, nil
}

// String returns a convenient display string for the source.
func (t Source) String() string {
	return sourceStr[t]
}
