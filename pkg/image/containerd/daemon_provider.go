package containerd

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/images/archive"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/platforms"
	"github.com/containerd/containerd/remotes/docker"
	"github.com/containerd/containerd/remotes/docker/config"
	"github.com/google/go-containerregistry/pkg/name"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/wagoodman/go-partybus"
	"github.com/wagoodman/go-progress"

	"github.com/anchore/stereoscope/internal/bus"
	"github.com/anchore/stereoscope/internal/log"
	"github.com/anchore/stereoscope/pkg/event"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/image"
	sdocker "github.com/anchore/stereoscope/pkg/image/docker"
)

var mb = math.Pow(2, 20)

// DaemonImageProvider is a image.Provider capable of fetching and representing a docker image from the containerd daemon API.
type DaemonImageProvider struct {
	imageStr  string
	tmpDirGen *file.TempDirGenerator
	client    *containerd.Client
	platform  *image.Platform
	namespace string
}

type daemonProvideProgress struct {
	EstimateProgress *progress.TimedProgress
	ExportProgress   *progress.Manual
	Stage            *progress.Stage
}

// NewProviderFromDaemon creates a new provider instance for a specific image that will later be cached to the given directory.
func NewProviderFromDaemon(imgStr string, tmpDirGen *file.TempDirGenerator, c *containerd.Client, platform *image.Platform, namespace string) (*DaemonImageProvider, error) {
	ref, err := name.ParseReference(imgStr, name.WithDefaultRegistry(""))
	if err != nil {
		return nil, err
	}
	tag, ok := ref.(name.Tag)
	if ok {
		imgStr = tag.Name()
	}

	if namespace == "" {
		namespace = namespaces.Default
	}

	return &DaemonImageProvider{
		imageStr:  imgStr,
		tmpDirGen: tmpDirGen,
		client:    c,
		platform:  platform,
		namespace: namespace,
	}, nil
}

// Provide an image object that represents the cached docker image tar fetched from a containerd daemon.
func (p *DaemonImageProvider) Provide(ctx context.Context, userMetadata ...image.AdditionalMetadata) (*image.Image, error) {
	ctx = namespaces.WithNamespace(ctx, p.namespace)

	resolvedImage, resolvedPlatform, err := p.pullImageIfMissing(ctx)
	if err != nil {
		return nil, err
	}

	tarFileName, err := p.saveImage(ctx, resolvedImage)
	if err != nil {
		return nil, err
	}

	// use the existing tarball provider to process what was pulled from the containerd daemon
	return sdocker.NewProviderFromTarball(tarFileName, p.tmpDirGen).Provide(ctx, withMetadata(resolvedPlatform, userMetadata)...)
}

// pull a containerd image
func (p *DaemonImageProvider) pull(ctx context.Context, resolvedImage string) (containerd.Image, error) {
	var platformStr string
	if p.platform != nil {
		platformStr = p.platform.String()
	}

	log.WithFields("image", resolvedImage, "platform", platformStr).Debug("pulling containerd")

	ongoing := newJobs(resolvedImage)

	// publish a pull event on the bus, allowing for read-only consumption of status
	bus.Publish(partybus.Event{
		Type:   event.PullContainerdImage,
		Source: resolvedImage,
		Value:  newPullStatus(p.client, ongoing).start(ctx),
	})

	h := images.HandlerFunc(func(ctx context.Context, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
		// as new layers (and other artifacts) are discovered, add them to the ongoing list of things to monitor while pulling
		if desc.MediaType != images.MediaTypeDockerSchema1Manifest {
			ongoing.Add(desc)
		}
		return nil, nil
	})

	options := p.pullOptions(ctx)
	options = append(options, containerd.WithImageHandler(h))
	if platformStr != "" {
		options = append(options, containerd.WithPlatform(platformStr))
	}

	// note: this will return an image object with the platform correctly set (if it exists)
	resp, err := p.client.Pull(ctx, resolvedImage, options...)
	if err != nil {
		return nil, fmt.Errorf("pull failed: %w", err)
	}

	return resp, nil
}

func (p *DaemonImageProvider) pullOptions(ctx context.Context) []containerd.RemoteOpt {
	var options = []containerd.RemoteOpt{
		containerd.WithPlatform(p.platform.String()),
	}

	doptions := docker.ResolverOptions{
		Tracker: docker.NewInMemoryTracker(),
	}

	useRegAuth, err := strconv.ParseBool(os.Getenv("USE_REGISTRY_AUTH"))
	if err != nil {
		useRegAuth = false
	}

	if useRegAuth {
		username := os.Getenv("CTR_REGISTRY_USERNAME")
		secret := os.Getenv("CTR_REGISTRY_PASSWORD")
		hostOptions := config.HostOptions{
			Credentials: func(host string) (string, string, error) {
				return username, secret, nil
			},
		}

		regUseHTTP, err := strconv.ParseBool(os.Getenv("REGISTRY_USE_HTTP"))
		if err != nil {
			regUseHTTP = false
		}

		if regUseHTTP {
			hostOptions.DefaultScheme = "http"
		} else {
			hostOptions.DefaultScheme = "https"
		}

		doptions.Hosts = config.ConfigureHosts(ctx, hostOptions)
	}

	options = append(options, containerd.WithResolver(docker.NewResolver(doptions)))

	return options
}

func (p *DaemonImageProvider) resolveImage(ctx context.Context, imageStr string) (string, *platforms.Platform, error) {
	// check if the image exists locally

	// note: you can NEVER depend on the GetImage() call to return an object with a platform set (even if you specify
	// a reference to a specific manifest via digest... not a digest for a manifest list!).
	img, err := p.client.GetImage(ctx, imageStr)
	if err != nil {
		// no image found
		return imageStr, nil, err
	}

	if p.platform == nil {
		// the user is not asking for a platform-specific request -- return what containerd returns
		return imageStr, nil, nil
	}

	processManifest := func(imageStr string, manifestDesc ocispec.Descriptor) (string, *platforms.Platform, error) {
		manifest, err := p.fetchManifest(ctx, manifestDesc)
		if err != nil {
			return "", nil, err
		}

		platform, err := p.fetchPlatformFromConfig(ctx, manifest.Config)
		if err != nil {
			return "", nil, err
		}

		return imageStr, platform, nil
	}

	// let's make certain that the image we found is for the platform we want (but the hard way!)
	desc := img.Target()
	switch desc.MediaType {
	case images.MediaTypeDockerSchema2Manifest, ocispec.MediaTypeImageManifest:
		return processManifest(imageStr, desc)

	case images.MediaTypeDockerSchema2ManifestList, ocispec.MediaTypeImageIndex:
		img = nil

		// let's find the digest for the manifest for the specific architecture we want
		by, err := content.ReadBlob(ctx, p.client.ContentStore(), desc)
		if err != nil {
			return "", nil, fmt.Errorf("unable to fetch manifest list: %w", err)
		}

		var index ocispec.Index
		if err := json.Unmarshal(by, &index); err != nil {
			return "", nil, fmt.Errorf("unable to unmarshal manifest list: %w", err)
		}

		platformObj, err := platforms.Parse(p.platform.String())
		if err != nil {
			return "", nil, fmt.Errorf("unable to parse platform: %w", err)
		}
		platformMatcher := platforms.NewMatcher(platformObj)
		for _, manifestDesc := range index.Manifests {
			if manifestDesc.Platform == nil {
				continue
			}
			if platformMatcher.Match(*manifestDesc.Platform) {
				return processManifest(imageStr, manifestDesc)
			}
		}

		// no manifest found for the platform we want
		return imageStr, nil, fmt.Errorf("no manifest found in manifest list for platform %q", p.platform.String())
	}

	return "", nil, fmt.Errorf("unexpected mediaType for image: %q", desc.MediaType)
}

func (p *DaemonImageProvider) fetchManifest(ctx context.Context, desc ocispec.Descriptor) (*ocispec.Manifest, error) {
	switch desc.MediaType {
	case images.MediaTypeDockerSchema2Manifest, ocispec.MediaTypeImageManifest:
		// pass
	default:
		return nil, fmt.Errorf("unexpected mediaType for image manifest: %q", desc.MediaType)
	}

	by, err := content.ReadBlob(ctx, p.client.ContentStore(), desc)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch image manifest: %w", err)
	}

	var manifest ocispec.Manifest
	if err := json.Unmarshal(by, &manifest); err != nil {
		return nil, fmt.Errorf("unable to unmarshal image manifest: %w", err)
	}

	return &manifest, nil
}

func (p *DaemonImageProvider) fetchPlatformFromConfig(ctx context.Context, desc ocispec.Descriptor) (*platforms.Platform, error) {
	switch desc.MediaType {
	case images.MediaTypeDockerSchema2Config, ocispec.MediaTypeImageConfig:
		// pass
	default:
		return nil, fmt.Errorf("unexpected mediaType for image config: %q", desc.MediaType)
	}

	by, err := content.ReadBlob(ctx, p.client.ContentStore(), desc)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch image config: %w", err)
	}

	var cfg ocispec.Platform
	if err := json.Unmarshal(by, &cfg); err != nil {
		return nil, fmt.Errorf("unable to unmarshal platform info from image config: %w", err)
	}

	return &cfg, nil
}

func (p *DaemonImageProvider) pullImageIfMissing(ctx context.Context) (string, *platforms.Platform, error) {
	p.imageStr = checkRegistryHostMissing(p.imageStr)

	// try to get the image first before pulling
	resolvedImage, resolvedPlatform, err := p.resolveImage(ctx, p.imageStr)

	imageStr := resolvedImage
	if imageStr == "" {
		imageStr = p.imageStr
	}

	if err != nil {
		_, err := p.pull(ctx, imageStr)
		if err != nil {
			return "", nil, err
		}

		resolvedImage, resolvedPlatform, err = p.resolveImage(ctx, imageStr)
		if err != nil {
			return "", nil, fmt.Errorf("unable to resolve image after pull: %w", err)
		}
	}

	if err := p.validatePlatform(resolvedPlatform); err != nil {
		return "", nil, fmt.Errorf("platform validation failed: %w", err)
	}

	return resolvedImage, resolvedPlatform, nil
}

func (p *DaemonImageProvider) validatePlatform(platform *platforms.Platform) error {
	if p.platform == nil {
		return nil
	}

	if platform == nil {
		return fmt.Errorf("image has no platform information (might be a manifest list)")
	}

	if platform.OS != p.platform.OS {
		return fmt.Errorf("image has unexpected OS %q, which differs from the user specified PS %q", platform.OS, p.platform.OS)
	}

	if platform.Architecture != p.platform.Architecture {
		return fmt.Errorf("image has unexpected architecture %q, which differs from the user specified architecture %q", platform.Architecture, p.platform.Architecture)
	}

	if platform.Variant != p.platform.Variant {
		return fmt.Errorf("image has unexpected architecture %q, which differs from the user specified architecture %q", platform.Variant, p.platform.Variant)
	}

	return nil
}

// save the image from the containerd daemon to a tar file
func (p *DaemonImageProvider) saveImage(ctx context.Context, resolvedImage string) (string, error) {
	imageTempDir, err := p.tmpDirGen.NewDirectory("containerd-daemon-image")
	if err != nil {
		return "", err
	}

	// create a file within the temp dir
	tempTarFile, err := os.Create(path.Join(imageTempDir, "image.tar"))
	if err != nil {
		return "", fmt.Errorf("unable to create temp file for image: %w", err)
	}
	defer func() {
		err := tempTarFile.Close()
		if err != nil {
			log.Errorf("unable to close temp file (%s): %w", tempTarFile.Name(), err)
		}
	}()

	is := p.client.ImageService()
	exportOpts := []archive.ExportOpt{
		archive.WithImage(is, resolvedImage),
	}

	img, err := p.client.GetImage(ctx, resolvedImage)
	if err != nil {
		return "", fmt.Errorf("unable to fetch image from containerd: %w", err)
	}

	size, err := img.Size(ctx)
	if err != nil {
		log.WithFields("error", err).Trace("unable to fetch image size from containerd, progress will not be tracked accurately")
		size = int64(50 * mb)
	}

	if p.platform != nil {
		platformObj, err := platforms.Parse(p.platform.String())
		if err != nil {
			return "", fmt.Errorf("unable to parse platform: %w", err)
		}
		// important: we require OnlyStrict() to ensure that when arm64 is provided that other arm variants are NOT selected
		exportOpts = append(exportOpts, archive.WithPlatform(platforms.OnlyStrict(platformObj)))
	}

	providerProgress := p.trackSaveProgress(size)
	defer func() {
		// NOTE: progress trackers should complete at the end of this function
		// whether the function errors or succeeds.
		providerProgress.EstimateProgress.SetCompleted()
		providerProgress.ExportProgress.SetCompleted()
	}()

	providerProgress.Stage.Current = "requesting image from containerd"

	// containerd export (save) does not return till fully complete
	err = p.client.Export(ctx, tempTarFile, exportOpts...)
	if err != nil {
		return "", fmt.Errorf("unable to save image tar for image=%q: %w", img.Name(), err)
	}

	return tempTarFile.Name(), nil
}

func (p *DaemonImageProvider) trackSaveProgress(size int64) *daemonProvideProgress {
	// docker image save clocks in at ~40MB/sec on my laptop... mileage may vary, of course :shrug:
	sec := float64(size) / (mb * 40)
	approxSaveTime := time.Duration(sec*1000) * time.Millisecond

	estimateSaveProgress := progress.NewTimedProgress(approxSaveTime)
	exportProgress := progress.NewManual(1)
	aggregateProgress := progress.NewAggregator(progress.DefaultStrategy, estimateSaveProgress, exportProgress)

	// let consumers know of a monitorable event (image save + copy stages)
	stage := &progress.Stage{}

	bus.Publish(partybus.Event{
		Type:   event.FetchImage,
		Source: p.imageStr,
		Value: progress.StagedProgressable(&struct {
			progress.Stager
			progress.Progressable
		}{
			Stager:       progress.Stager(stage),
			Progressable: aggregateProgress,
		}),
	})

	return &daemonProvideProgress{
		EstimateProgress: estimateSaveProgress,
		ExportProgress:   exportProgress,
		Stage:            stage,
	}
}

func withMetadata(platform *platforms.Platform, userMetadata []image.AdditionalMetadata) (metadata []image.AdditionalMetadata) {
	// TODO: there might be a way to fetch tags for the image

	if platform != nil {
		metadata = append(metadata,
			image.WithArchitecture(platform.Architecture, platform.Variant),
			image.WithOS(platform.OS),
		)
	}

	// apply user-supplied metadata last to override any default behavior
	metadata = append(metadata, userMetadata...)
	return metadata
}

// if image doesn't have host set, add docker hub by default
func checkRegistryHostMissing(imageName string) string {
	parts := strings.Split(imageName, "/")
	if len(parts) == 1 {
		return fmt.Sprintf("docker.io/library/%s", imageName)
	} else if len(parts) > 1 && !strings.Contains(parts[0], ".") {
		return fmt.Sprintf("docker.io/%s", imageName)
	}
	return imageName
}
