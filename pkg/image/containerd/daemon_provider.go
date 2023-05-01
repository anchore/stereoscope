package containerd

import (
	"context"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/images/archive"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/platforms"
	"github.com/containerd/containerd/remotes/docker"
	"github.com/containerd/containerd/remotes/docker/config"
	"github.com/google/go-containerregistry/pkg/name"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/wagoodman/go-partybus"

	"github.com/anchore/stereoscope/internal/bus"
	"github.com/anchore/stereoscope/internal/log"
	"github.com/anchore/stereoscope/pkg/event"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/image"
	sdocker "github.com/anchore/stereoscope/pkg/image/docker"
)

var (
	defaultNamespace = namespaces.Default
)

// DaemonImageProvider is a image.Provider capable of fetching and representing a docker image from the containerd daemon API.
type DaemonImageProvider struct {
	imageStr  string
	tmpDirGen *file.TempDirGenerator
	client    *containerd.Client
	platform  *image.Platform
}

// NewProviderFromDaemon creates a new provider instance for a specific image that will later be cached to the given directory.
func NewProviderFromDaemon(imgStr string, tmpDirGen *file.TempDirGenerator, c *containerd.Client, platform *image.Platform) (*DaemonImageProvider, error) {
	ref, err := name.ParseReference(imgStr, name.WithDefaultRegistry(""))
	if err != nil {
		return nil, err
	}
	tag, ok := ref.(name.Tag)
	if ok {
		imgStr = tag.Name()
	}

	return &DaemonImageProvider{
		imageStr:  imgStr,
		tmpDirGen: tmpDirGen,
		client:    c,
		platform:  platform,
	}, nil
}

// pull a containerd image
func (p *DaemonImageProvider) pull(ctx context.Context) (containerd.Image, error) {
	log.Debugf("pulling containerd image=%q", p.imageStr)

	ongoing := newJobs(p.imageStr)

	// publish a pull event on the bus, allowing for read-only consumption of status
	bus.Publish(partybus.Event{
		Type:   event.PullContainerdImage,
		Source: p.imageStr,
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

	resp, err := p.client.Pull(ctx, p.imageStr, options...)
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

// Provide an image object that represents the cached docker image tar fetched from a containerd daemon.
func (p *DaemonImageProvider) Provide(ctx context.Context, userMetadata ...image.AdditionalMetadata) (*image.Image, error) {
	ctx = namespaces.WithNamespace(ctx, defaultNamespace)
	image, err := p.pullImageIfMissing(ctx)
	if err != nil {
		return nil, err
	}

	if err := p.validatePlatform(*image); err != nil {
		return nil, err
	}

	tarFileName, err := p.saveImage(ctx, *image)
	if err != nil {
		return nil, err
	}

	// use the existing tarball provider to process what was pulled from the containerd daemon
	return sdocker.NewProviderFromTarball(tarFileName, p.tmpDirGen).Provide(ctx, withMetadata(*image, userMetadata)...)
}

func (p *DaemonImageProvider) pullImageIfMissing(ctx context.Context) (*containerd.Image, error) {
	p.imageStr = checkRegistryHostMissing(p.imageStr)

	// check if the image exists locally
	img, err := p.client.GetImage(ctx, p.imageStr)
	if err != nil {
		//TODO: include platform in pulling the image
		pulledImaged, err := p.pull(ctx)
		if err != nil {
			return nil, err
		}
		return &pulledImaged, nil
	}

	// looks like the image exists, but the platform doesn't match what the user specified, so we need to
	// pull the image again with the correct platform specifier, which will override the local tag.
	if err := p.validatePlatform(img); err != nil {
		//TODO: include platform in pulling the image
		pulledImaged, err := p.pull(ctx)
		if err != nil {
			return nil, err
		}
		return &pulledImaged, nil
	}

	return &img, nil
}

func (p *DaemonImageProvider) validatePlatform(img containerd.Image) error {
	if p.platform == nil || img.Target().Platform == nil {
		// the user did not specify a platform
		return nil
	}

	platform := img.Target().Platform

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
func (p *DaemonImageProvider) saveImage(ctx context.Context, img containerd.Image) (string, error) {
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
		archive.WithImage(is, img.Name()),
		archive.WithPlatform(platforms.DefaultStrict()),
	}

	// containerd export (save) does not return till fully complete
	err = p.client.Export(ctx, tempTarFile, exportOpts...)
	if err != nil {
		return "", fmt.Errorf("unable to save image tar: %w", err)
	}

	return tempTarFile.Name(), nil
}

func withMetadata(img containerd.Image, userMetadata []image.AdditionalMetadata) (metadata []image.AdditionalMetadata) {
	tags := []string{}
	for k, v := range img.Labels() {
		tags = append(tags, fmt.Sprintf("%s:%s", k, v))
	}

	platform := img.Target().Platform

	if platform != nil {
		metadata = append(metadata,
			image.WithTags(tags...),
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
