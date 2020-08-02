package docker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"strings"
	"time"

	"github.com/anchore/stereoscope/internal/bus"
	"github.com/anchore/stereoscope/internal/docker"
	"github.com/anchore/stereoscope/internal/log"
	"github.com/anchore/stereoscope/pkg/event"
	"github.com/anchore/stereoscope/pkg/image"
	"github.com/docker/cli/cli/config"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/wagoodman/go-partybus"
	"github.com/wagoodman/go-progress"
)

// DaemonImageProvider is a image.Provider capable of fetching and representing a docker image from the docker daemon API.
type DaemonImageProvider struct {
	ImageRef name.Reference
	cacheDir string
}

// NewProviderFromDaemon creates a new provider instance for a specific image that will later be cached to the given directory.
func NewProviderFromDaemon(imgRef name.Reference, cacheDir string) *DaemonImageProvider {
	return &DaemonImageProvider{
		ImageRef: imgRef,
		cacheDir: cacheDir,
	}
}

func (p *DaemonImageProvider) trackSaveProgress() (*progress.TimedProgress, *progress.Writer, *progress.Stage, error) {
	dockerClient, err := docker.GetClient()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("unable to get docker client: %w", err)
	}

	// fetch the expected image size to estimate and measure progress
	inspect, _, err := dockerClient.ImageInspectWithRaw(context.Background(), p.ImageRef.Name())
	if err != nil {
		return nil, nil, nil, fmt.Errorf("unable to inspect image: %w", err)
	}

	// docker image save clocks in at ~125MB/sec on my laptop... milage may vary, of course :shrug:
	mb := math.Pow(2, 20)
	sec := float64(inspect.VirtualSize) / (mb * 125)
	approxSaveTime := time.Duration(sec*1000) * time.Millisecond

	estimateSaveProgress := progress.NewTimedProgress(approxSaveTime)
	copyProgress := progress.NewSizedWriter(inspect.VirtualSize)
	aggregateProgress := progress.NewAggregator(progress.NormalizeStrategy, estimateSaveProgress, copyProgress)

	// let consumers know of a monitorable event (image save + copy stages)
	stage := &progress.Stage{}

	bus.Publish(partybus.Event{
		Type:   event.FetchImage,
		Source: p.ImageRef.Name(),
		Value: progress.StagedProgressable(&struct {
			progress.Stager
			*progress.Aggregator
		}{
			Stager:     progress.Stager(stage),
			Aggregator: aggregateProgress,
		}),
	})

	return estimateSaveProgress, copyProgress, stage, nil
}

// pull a docker image
func (p *DaemonImageProvider) pull(ctx context.Context) error {
	log.Debugf("pulling docker image=%q", p.ImageRef.String())

	// note: this will search the default config dir and allow for a DOCKER_CONFIG override
	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("failed to load docker config: %w", err)
	}
	log.Debugf("using docker config=%q", cfg.Filename)

	hostname := p.ImageRef.Context().RegistryStr()
	creds, err := cfg.GetAuthConfig(hostname)
	if err != nil {
		return fmt.Errorf("failed to fetch registry auth (hostname=%s): %w", hostname, err)
	}

	dockerClient, err := docker.GetClient()
	if err != nil {
		return fmt.Errorf("failed to load docker client: %w", err)
	}

	var opts types.ImagePullOptions

	if creds.Username != "" {
		log.Debugf("using docker credentials for %q", hostname)
		jsonBytes, _ := json.Marshal(map[string]string{
			"username": creds.Username,
			"password": creds.Password,
		})
		opts.RegistryAuth = base64.StdEncoding.EncodeToString(jsonBytes)
	}

	var status = newPullStatus()
	defer func() {
		status.complete = true
	}()

	// publish a pull event on the bus, allowing for read-only consumption of status
	bus.Publish(partybus.Event{
		Type:   event.PullDockerImage,
		Source: p.ImageRef.Name(),
		Value:  status,
	})

	resp, err := dockerClient.ImagePull(ctx, p.ImageRef.Name(), opts)
	if err != nil {
		return fmt.Errorf("pull failed: %w", err)
	}

	var thePullEvent *pullEvent
	decoder := json.NewDecoder(resp)
	for {
		if err := decoder.Decode(&thePullEvent); err != nil {
			if err == io.EOF {
				break
			}

			return fmt.Errorf("failed to pull image: %w", err)
		}

		// check for the last two events indicating the pull is complete
		if strings.HasPrefix(thePullEvent.Status, "Digest:") || strings.HasPrefix(thePullEvent.Status, "Status:") {
			continue
		}

		status.onEvent(thePullEvent)
	}

	return nil
}

// Provide an image object that represents the cached docker image tar fetched from a docker daemon.
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

	// check if the image exists, if not pull it...
	_, _, err = dockerClient.ImageInspectWithRaw(context.Background(), p.ImageRef.Name())
	if err != nil {
		if client.IsErrNotFound(err) {
			if err = p.pull(context.Background()); err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("unable to inspect existing image: %w", err)
		}
	}

	// save the image from the docker daemon to a tar file
	estimateSaveProgress, copyProgress, stage, err := p.trackSaveProgress()
	if err != nil {
		return nil, fmt.Errorf("unable to trace image save progress: %w", err)
	}

	stage.Current = "requesting image from docker"
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

	// cancel indeterminate progress
	estimateSaveProgress.SetCompleted()

	// save the image contents to the temp file
	// note: this is the same image that will be used to querying image content during analysis
	stage.Current = "saving image to disk"
	nBytes, err := io.Copy(io.MultiWriter(tempTarFile, copyProgress), readCloser)
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
