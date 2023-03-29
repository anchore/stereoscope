package image

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/anchore/stereoscope/internal/docker"
	"github.com/anchore/stereoscope/internal/log"
	"github.com/anchore/stereoscope/internal/podman"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/mitchellh/go-homedir"
	"github.com/spf13/afero"
	"github.com/sylabs/sif/v2/pkg/sif"
)

const (
	UnknownSource Source = iota
	DockerTarballSource
	DockerDaemonSource
	OciDirectorySource
	OciTarballSource
	OciRegistrySource
	PodmanDaemonSource
	SingularitySource
)

const SchemeSeparator = ":"

var sourceStr = [...]string{
	"UnknownSource",
	"DockerTarball",
	"DockerDaemon",
	"OciDirectory",
	"OciTarball",
	"OciRegistry",
	"PodmanDaemon",
	"Singularity",
}

var AllSources = []Source{
	DockerTarballSource,
	DockerDaemonSource,
	OciDirectorySource,
	OciTarballSource,
	OciRegistrySource,
	PodmanDaemonSource,
	SingularitySource,
}

// Source is a concrete a selection of valid concrete image providers.
type Source uint8

// isRegistryReference takes a string and indicates if it conforms to a container image reference.
func isRegistryReference(imageSpec string) bool {
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
	case "podman":
		return PodmanDaemonSource
	case "oci-dir":
		return OciDirectorySource
	case "oci-archive":
		return OciTarballSource
	case "oci-registry", "registry":
		return OciRegistrySource
	case "singularity":
		return SingularitySource
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

	var source = UnknownSource
	var location = userInput
	var sourceHint string
	var err error
	if len(candidates) == 2 {
		// the user may have provided a source hint (or this is a split from a path or docker image reference, we aren't certain yet)
		sourceHint = candidates[0]
		source = ParseSourceScheme(sourceHint)
	}
	if source != UnknownSource {
		// if we found source from hint, than remove the hint from the location
		location = strings.TrimPrefix(userInput, sourceHint+SchemeSeparator)
	} else {
		// a valid source hint wasnt provided/detected, try detect one
		source, err = detectSourceFromPath(fs, location)
		if err != nil {
			return UnknownSource, "", err
		}
	}

	switch source {
	case OciDirectorySource, OciTarballSource, DockerTarballSource, SingularitySource:
		// since the scheme was explicitly given, that means that home dir tilde expansion would not have been done by the shell (so we have to)
		location, err = homedir.Expand(location)
		if err != nil {
			return UnknownSource, "", fmt.Errorf("unable to expand potential home dir expression: %w", err)
		}
	case UnknownSource:
		location = ""
	}

	return source, location, nil
}

func checkImageDaemon(f func() (*client.Client, error)) error {
	c, err := f()
	if err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		pong, err := c.Ping(ctx)
		if err == nil && pong.APIVersion != "" {
			// the daemon exists and is accessible
			// return the desired source
			return nil
		}
		return err
	}
	return err
}

// DetermineDefaultImagePullSource takes an image reference string as input
// along with an ordered list of sources to check. It uses these to determine a
// source to use to pull the image. If the input doesn't specify an
// image reference (i.e. an image that can be _pulled_), UnknownSource is
// returned. Otherwise, if the Docker daemon is available, DockerDaemonSource is
// returned, and if not, OciRegistrySource is returned.
func DetermineDefaultImagePullSource(userInput string, sources []Source) Source {
	if !isRegistryReference(userInput) {
		return UnknownSource
	}

	// check based on preferred order first
	for _, source := range sources {
		switch source {
		case DockerDaemonSource:
			// verify that the Docker daemon is
			// accessible before assuming we can use it
			err := checkImageDaemon(docker.GetClient)
			if err != nil {
				log.Tracef("docker daemon not available: %w", err)
				continue
			}
			return DockerDaemonSource
		// verify that the Podman daemon is
		// accessible before assuming we can use it
		case PodmanDaemonSource:
			err := checkImageDaemon(podman.GetClient)
			if err != nil {
				log.Tracef("podman daemon not available: %w", err)
				continue
			}
			return PodmanDaemonSource
		case OciRegistrySource:
			return OciRegistrySource
		}
	}

	// daemons could not be accessed and OciRegistrySource was not a given option
	return UnknownSource
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

	f, err := fs.Open(imgPath)
	if err != nil {
		return UnknownSource, fmt.Errorf("unable to open file=%s: %w", imgPath, err)
	}
	defer f.Close()

	// Check for Singularity container.
	fi, err := sif.LoadContainer(f, sif.OptLoadWithCloseOnUnload(false))
	if err == nil {
		return SingularitySource, fi.UnloadContainer()
	}

	// assume this is an archive...
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
		if _, err = f.Seek(0, io.SeekStart); err != nil {
			return UnknownSource, fmt.Errorf("unable to seek archive=%s: %w", imgPath, err)
		}

		var fileErr *file.ErrFileNotFound
		_, err = file.ReaderFromTar(f, pair.path)
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
