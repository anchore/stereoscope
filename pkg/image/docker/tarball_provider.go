package docker

import (
	"archive/tar"
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/go-containerregistry/pkg/name"
	"io"
	"os"
	"path"
	"path/filepath"

	"github.com/anchore/stereoscope/internal/log"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/image"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

var ErrMultipleManifests = fmt.Errorf("cannot process multiple docker manifests")

// TarballImageProvider is a image.Provider for a docker image (V2) for an existing tar on disk (the output from a "docker image save ..." command).
type TarballImageProvider struct {
	path      string
	tmpDirGen *file.TempDirGenerator
}

// NewProviderFromTarball creates a new provider instance for the specific image already at the given path.
func NewProviderFromTarball(path string, tmpDirGen *file.TempDirGenerator) *TarballImageProvider {
	return &TarballImageProvider{
		path:      path,
		tmpDirGen: tmpDirGen,
	}
}

// Provide an image object that represents the docker image tar at the configured location on disk.
func (p *TarballImageProvider) Provide(_ context.Context, userMetadata ...image.AdditionalMetadata) (*image.Image, error) {
	img, err := tarball.ImageFromPath(p.path, nil)
	if err != nil {
		// raise a more controlled error for when there are multiple images within the given tar (from https://github.com/anchore/grype/issues/215)
		if err.Error() == "tarball must contain only a single image to be used with tarball.Image" {
			images, err := multiImage(pathOpener(p.path))
			if err != nil {
				// TODO: nope
				panic(err)
			}

			/// TODO: now what? we can't reconcile a single image out

			//return nil, ErrMultipleManifests
		}
		return nil, fmt.Errorf("unable to provide image from tarball: %w", err)
	}

	// make a best-effort to generate an OCI manifest and gets tags, but ultimately this should be considered optional
	var rawOCIManifest []byte
	var rawConfig []byte
	var ociManifest *v1.Manifest
	var metadata []image.AdditionalMetadata

	theManifest, err := extractManifest(p.path)
	if err != nil {
		log.Warnf("could not extract manifest: %+v", err)
	}

	if theManifest != nil {
		// given that we have a manifest, continue processing to get the tags and OCI manifest
		metadata = append(metadata, image.WithTags(theManifest.allTags()...))

		ociManifest, rawConfig, err = generateOCIManifest(p.path, theManifest)
		if err != nil {
			log.Warnf("failed to generate OCI manifest from docker archive: %+v", err)
		}

		// we may have the config available, use it
		if rawConfig != nil {
			metadata = append(metadata, image.WithConfig(rawConfig))
		}
	}

	if ociManifest != nil {
		rawOCIManifest, err = json.Marshal(&ociManifest)
		if err != nil {
			log.Warnf("failed to serialize OCI manifest: %+v", err)
		} else {
			metadata = append(metadata, image.WithManifest(rawOCIManifest))
		}
	}

	// apply user-supplied metadata last to override any default behavior
	metadata = append(metadata, userMetadata...)

	contentTempDir, err := p.tmpDirGen.NewDirectory("docker-tarball-image")
	if err != nil {
		return nil, err
	}

	return image.NewImage(img, contentTempDir, metadata...), nil
}

// code extracted from open PR: https://github.com/google/go-containerregistry/pull/1110

func multiImage(opener tarball.Opener, opt ...name.Option) (map[name.Reference]v1.Image, error) {
	m, err := extractFileFromTar(opener, "manifest.json")
	if err != nil {
		return nil, err
	}
	var iManifest = make(tarball.Manifest, 0)
	defer m.Close()
	if err = json.NewDecoder(m).Decode(&iManifest); err != nil {
		return nil, err
	}
	var out = make(map[name.Reference]v1.Image, len(iManifest))
	for i := range iManifest {
		manifest := iManifest[i]
		if len(manifest.RepoTags) == 0 {
			return nil, fmt.Errorf("repo tags missing from manifest config %s", manifest.Config)
		}
		// TODO: does the first entry make sense?
		ref, err := name.ParseReference(manifest.RepoTags[0], opt...)
		if err != nil {
			return nil, err
		}
		t, _ := name.NewTag(ref.String())
		if err != nil {
			return nil, err
		}
		v1Image, err := tarball.Image(opener, &t)

		if err != nil {
			return nil, err
		}
		out[ref] = v1Image
	}
	return out, nil
}

func pathOpener(path string) tarball.Opener {
	return func() (io.ReadCloser, error) {
		return os.Open(path)
	}
}

func extractFileFromTar(opener tarball.Opener, filePath string) (io.ReadCloser, error) {
	f, err := opener()
	if err != nil {
		return nil, err
	}
	close := true
	defer func() {
		if close {
			f.Close()
		}
	}()

	tf := tar.NewReader(f)
	for {
		hdr, err := tf.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Name == filePath {
			if hdr.Typeflag == tar.TypeSymlink || hdr.Typeflag == tar.TypeLink {
				currentDir := filepath.Dir(filePath)
				return extractFileFromTar(opener, path.Join(currentDir, hdr.Linkname))
			}
			close = false
			return tarFile{
				Reader: tf,
				Closer: f,
			}, nil
		}
	}
	return nil, fmt.Errorf("file %s not found in tar", filePath)
}

// tarFile represents a single file inside a tar. Closing it closes the tar itself.
type tarFile struct {
	io.Reader
	io.Closer
}
