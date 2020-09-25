package image

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/mitchellh/go-homedir"
	"github.com/spf13/afero"
)

const (
	UnknownSource Source = iota
	DockerTarballSource
	DockerDaemonSource
	OciDirectorySource
	OciTarballSource
)

const SchemeSeparator = ":"

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

// isDockerReference takes a string and indicates if it conforms to a docker image reference.
func isDockerReference(imageSpec string) bool {
	// note: strict validation requires there to be a default registry (e.g. docker.io) which we cannot assume will be provided
	// we only want to validate the bare minimum number of image specification features, not exhaustive.
	_, err := name.ParseReference(imageSpec, name.WeakValidation)
	return err == nil
}

// ParseSourceScheme attempts to resolve a concrete image source selection from a scheme in a user string.
func ParseSourceScheme(source string) Source {
	source = strings.ToLower(source)
	switch source {
	case "docker-archive":
		return DockerTarballSource
	case "docker":
		return DockerDaemonSource
	case "oci-dir":
		return OciDirectorySource
	case "oci-archive":
		return OciTarballSource
	}
	return UnknownSource
}

// DetectSource takes a user string and determines the image source (e.g. the docker daemon, a tar file, etc.) returning the string subset representing the image (or nothing if it is unknown).
// note: parsing is done relative to the given string and environmental evidence (i.e. the given filesystem) to determine the actual source.
func DetectSource(userInput string) (Source, string, error) {
	return detectSource(afero.NewOsFs(), userInput)
}

// DetectSource takes a user string and determines the image source (e.g. the docker daemon, a tar file, etc.) returning the string subset representing the image (or nothing if it is unknown).
// note: parsing is done relative to the given string and environmental evidence (i.e. the given filesystem) to determine the actual source.
func detectSource(fs afero.Fs, userInput string) (Source, string, error) {
	candidates := strings.SplitN(userInput, SchemeSeparator, 2)

	var source Source
	var location = userInput
	var sourceHint string
	var err error
	switch len(candidates) {
	case 1:
		// no source hint has been provided, detect one
		source, err = detectSourceFromPath(fs, location)
		if err != nil {
			return UnknownSource, "", err
		}
	case 2:
		// the user may have provided a source hint (or this is a split from a docker image reference, we aren't certain yet)
		sourceHint = candidates[0]
		location = strings.TrimPrefix(userInput, sourceHint+SchemeSeparator)
		source = ParseSourceScheme(sourceHint)
	default:
		source = UnknownSource
	}

	switch source {
	case OciDirectorySource, OciTarballSource, DockerTarballSource:
		// since the scheme was explicitly given, that means that home dir tilde expansion would not have been done by the shell (so we have to)
		location, err = homedir.Expand(location)
		if err != nil {
			return UnknownSource, "", fmt.Errorf("unable to expand potential home dir expression: %w", err)
		}
	case UnknownSource:
		if isDockerReference(userInput) {
			// ignore any source hint since the source is ultimately unknown, see if this could be a docker image
			return DockerDaemonSource, userInput, nil
		}
		// invalidate any previous processing if the source is still unknown
		location = ""
	}

	return source, location, nil
}

// DetectSourceFromPath will distinguish between a oci-layout dir, oci-archive, and a docker-archive for a given filesystem.
func DetectSourceFromPath(imgPath string) (Source, error) {
	return detectSourceFromPath(afero.NewOsFs(), imgPath)
}

// detectSourceFromPath will distinguish between a oci-layout dir, oci-archive, and a docker-archive for a given filesystem.
func detectSourceFromPath(fs afero.Fs, imgPath string) (Source, error) {
	imgPath, err := homedir.Expand(imgPath)
	if err != nil {
		return UnknownSource, fmt.Errorf("unable to expand potential home dir expression: %w", err)
	}

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
