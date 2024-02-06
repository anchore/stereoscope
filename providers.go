package stereoscope

import (
	containerdClient "github.com/anchore/stereoscope/internal/containerd"
	"github.com/anchore/stereoscope/pkg/image"
	"github.com/anchore/stereoscope/pkg/image/containerd"
	"github.com/anchore/stereoscope/pkg/image/docker"
	"github.com/anchore/stereoscope/pkg/image/oci"
	"github.com/anchore/stereoscope/pkg/image/podman"
	"github.com/anchore/stereoscope/pkg/image/sif"
	"github.com/anchore/stereoscope/tagged"
)

const (
	FileTag     = "file"
	DirTag      = "dir"
	DaemonTag   = "daemon"
	PullTag     = "pull"
	RegistryTag = "registry"
)

// ImageProviderConfig is the uber-configuration containing all configuration needed by stereoscope image providers
type ImageProviderConfig struct {
	Registry image.RegistryOptions
	Platform *image.Platform
}

func ImageProviders(cfg ImageProviderConfig) tagged.Values[image.Provider] {
	tempDirGenerator := rootTempDirGenerator.NewGenerator()

	return tagged.Values[image.Provider]{
		// file providers
		taggedProvider(docker.NewArchiveProvider(tempDirGenerator), FileTag),
		taggedProvider(oci.NewArchiveProvider(tempDirGenerator), FileTag),
		taggedProvider(oci.NewDirectoryProvider(tempDirGenerator), FileTag, DirTag),
		taggedProvider(sif.NewArchiveProvider(tempDirGenerator), FileTag),

		// daemon providers
		taggedProvider(docker.NewDaemonProvider(tempDirGenerator, cfg.Platform), DaemonTag, PullTag),
		taggedProvider(podman.NewDaemonProvider(tempDirGenerator, cfg.Platform), DaemonTag, PullTag),
		taggedProvider(containerd.NewDaemonProvider(tempDirGenerator, containerdClient.Namespace(), cfg.Registry, cfg.Platform), DaemonTag, PullTag),

		// registry providers
		taggedProvider(oci.NewRegistryProvider(tempDirGenerator, cfg.Registry, cfg.Platform), RegistryTag, PullTag),
	}
}

func taggedProvider(provider image.Provider, tags ...string) tagged.Value[image.Provider] {
	return tagged.New[image.Provider](provider, append([]string{provider.Name()}, tags...)...)
}
