package docker

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/anchore/stereoscope/internal/docker"
	"github.com/anchore/stereoscope/internal/log"
	"github.com/anchore/stereoscope/pkg/image"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

type DaemonImageProvider struct {
	ImageRef name.Reference
	cacheDir string
}

func NewProviderFromDaemon(imgRef name.Reference, cacheDir string) *DaemonImageProvider {
	return &DaemonImageProvider{
		ImageRef: imgRef,
		cacheDir: cacheDir,
	}
}

func (p *DaemonImageProvider) Provide() (*image.Image, error) {
	// create a file within the temp dir
	tempTarFile, err := os.Create(path.Join(p.cacheDir, "image.tar"))
	if err != nil {
		return nil, fmt.Errorf("unable to create temp file for image: %w", err)
	}
	defer func() {
		err := tempTarFile.Close()
		if err != nil {
			log.Errorf("unable to close temp file (%s): %w", tempTarFile.Name(), err)
		}
	}()

	// fetch the image from the docker daemon
	dockerClient, err := docker.GetClient()
	if err != nil {
		return nil, fmt.Errorf("unable to get docker client: %w", err)
	}

	readCloser, err := dockerClient.ImageSave(context.Background(), []string{p.ImageRef.Name()})
	if err != nil {
		return nil, fmt.Errorf("unable to save image tar: %w", err)
	}
	defer func() {
		err := readCloser.Close()
		if err != nil {
			log.Errorf("unable to close temp file (%s): %w", tempTarFile.Name(), err)
		}
	}()

	// save the image contents to the temp file
	// note: this is the same image that will be used to querying image content during analysis
	nBytes, err := io.Copy(tempTarFile, readCloser)
	if err != nil {
		return nil, fmt.Errorf("unable to save image to tar: %w", err)
	}
	if nBytes == 0 {
		return nil, fmt.Errorf("cannot provide an empty image")
	}

	// use the tar utils to load a v1.Image from the tar file on disk
	img, err := tarball.ImageFromPath(tempTarFile.Name(), nil)
	if err != nil {
		return nil, err
	}

	tags, err := extractTags(tempTarFile.Name())
	if err != nil {
		return nil, err
	}

	return image.NewImageWithTags(img, tags), nil
}
