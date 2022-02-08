package docker

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"strings"
	"time"

	"github.com/anchore/stereoscope/internal/bus"
	"github.com/anchore/stereoscope/internal/log"
	"github.com/anchore/stereoscope/pkg/event"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/image"
	"github.com/docker/cli/cli/config"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/wagoodman/go-partybus"
	"github.com/wagoodman/go-progress"
)

// DaemonImageProvider is a image.Provider capable of fetching and representing a docker image from the docker daemon API.
type DaemonImageProvider struct {
	imageStr  string
	tmpDirGen *file.TempDirGenerator
	client    *client.Client
}

// NewProviderFromDaemon creates a new provider instance for a specific image that will later be cached to the given directory.
func NewProviderFromDaemon(imgStr string, tmpDirGen *file.TempDirGenerator, c *client.Client) *DaemonImageProvider {
	return &DaemonImageProvider{
		imageStr:  imgStr,
		tmpDirGen: tmpDirGen,
		client:    c,
	}
}

func (p *DaemonImageProvider) trackSaveProgress() (*progress.TimedProgress, *progress.Writer, *progress.Stage, error) {
	// fetch the expected image size to estimate and measure progress
	inspect, _, err := p.client.ImageInspectWithRaw(context.Background(), p.imageStr)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("unable to inspect image: %w", err)
	}

	// docker image save clocks in at ~125MB/sec on my laptop... mileage may vary, of course :shrug:
	mb := math.Pow(2, 20)
	//"virtual size" is the total amount of disk-space used for the read-only image
	//data used by the container and the writable layer.
	//"size" (also provider by the inspect result) shows the amount of data (on disk)
	//that is used for the writable layer of each container.
	sec := float64(inspect.VirtualSize) / (mb * 125)
	approxSaveTime := time.Duration(sec*1000) * time.Millisecond

	estimateSaveProgress := progress.NewTimedProgress(approxSaveTime)
	copyProgress := progress.NewSizedWriter(inspect.VirtualSize)
	aggregateProgress := progress.NewAggregator(progress.NormalizeStrategy, estimateSaveProgress, copyProgress)

	// let consumers know of a monitorable event (image save + copy stages)
	stage := &progress.Stage{}

	bus.Publish(partybus.Event{
		Type:   event.FetchImage,
		Source: p.imageStr,
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
	log.Debugf("pulling docker image=%q", p.imageStr)

	// note: this will search the default config dir and allow for a DOCKER_CONFIG override
	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("failed to load docker config: %w", err)
	}
	log.Debugf("using docker config=%q", cfg.Filename)

	var status = newPullStatus()
	defer func() {
		status.complete = true
	}()

	// publish a pull event on the bus, allowing for read-only consumption of status
	bus.Publish(partybus.Event{
		Type:   event.PullDockerImage,
		Source: p.imageStr,
		Value:  status,
	})

	options, err := newPullOptions(p.imageStr, cfg)
	if err != nil {
		return err
	}

	resp, err := p.client.ImagePull(ctx, p.imageStr, options)
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
func (p *DaemonImageProvider) Provide(ctx context.Context, userMetadata ...image.AdditionalMetadata) (*image.Image, error) {
	imageTempDir, err := p.tmpDirGen.NewDirectory("docker-daemon-image")
	if err != nil {
		return nil, err
	}

	// create a file within the temp dir
	tempTarFile, err := os.Create(path.Join(imageTempDir, "image.tar"))
	if err != nil {
		return nil, fmt.Errorf("unable to create temp file for image: %w", err)
	}
	defer func() {
		err := tempTarFile.Close()
		if err != nil {
			log.Errorf("unable to close temp file (%s): %w", tempTarFile.Name(), err)
		}
	}()

	// check if the image exists locally
	inspectResult, _, err := p.client.ImageInspectWithRaw(ctx, p.imageStr)

	if err != nil {
		if client.IsErrNotFound(err) {
			if err = p.pull(ctx); err != nil {
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
	defer func() {
		// NOTE: progress trackers should complete at the end of this function
		// whether the function errors or succeeds.
		estimateSaveProgress.SetCompleted()
		copyProgress.SetComplete()
	}()

	// NOTE: The image save progress is only a guess (a timer counting up to a particular time where
	// the overall progress would be considered at 50%). It's logical to adjust the first image save timer
	// to complete when the image save operation returns. The defer statement is a fallback in case the numbers
	// from the docker daemon don't line up (as we saw when metadata and actual size differ)
	// or there is a problem that causes us to return early with an error.
	estimateSaveProgress.SetCompleted()

	stage.Current = "requesting image from docker"
	readCloser, err := p.client.ImageSave(ctx, []string{p.imageStr})
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
	stage.Current = "saving image to disk"
	nBytes, err := io.Copy(io.MultiWriter(tempTarFile, copyProgress), readCloser)
	if err != nil {
		return nil, fmt.Errorf("unable to save image to tar: %w", err)
	}
	if nBytes == 0 {
		return nil, errors.New("cannot provide an empty image")
	}

	var metadata []image.AdditionalMetadata

	if len(inspectResult.RepoTags) > 0 {
		metadata = append(metadata, image.WithTags(inspectResult.RepoTags...))
	}

	if len(inspectResult.RepoDigests) > 0 {
		metadata = append(metadata, image.WithRepoDigests(inspectResult.RepoDigests...))
	}

	// apply user-supplied metadata last to override any default behavior
	metadata = append(metadata, userMetadata...)

	// use the existing tarball provider to process what was pulled from the docker daemon
	return NewProviderFromTarball(tempTarFile.Name(), p.tmpDirGen).Provide(ctx, metadata...)
}

func newPullOptions(image string, cfg *configfile.ConfigFile) (types.ImagePullOptions, error) {
	var options types.ImagePullOptions

	ref, err := name.ParseReference(image)
	if err != nil {
		return options, err
	}

	hostname := ref.Context().RegistryStr()

	creds, err := cfg.GetAuthConfig(hostname)
	if err != nil {
		return options, fmt.Errorf("failed to fetch registry auth (hostname=%s): %w", hostname, err)
	}

	if creds.Username != "" {
		log.Debugf("using docker credentials for %q", hostname)

		options.RegistryAuth, err = encodeCredentials(creds.Username, creds.Password)
		if err != nil {
			return options, err
		}
	}

	return options, nil
}

func encodeCredentials(username, password string) (string, error) {
	buffer := &bytes.Buffer{}
	encoder := json.NewEncoder(buffer)
	// note: the contents may contain characters that should not be escaped (such as password contents)
	encoder.SetEscapeHTML(false)

	if err := encoder.Encode(map[string]string{
		"username": username,
		"password": password,
	}); err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(buffer.Bytes()), nil
}
