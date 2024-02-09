package podman

import (
	"github.com/docker/docker/client"

	"github.com/anchore/stereoscope/internal/podman"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/image"
	"github.com/anchore/stereoscope/pkg/image/docker"
)

const Daemon image.Source = image.PodmanDaemonSource

func NewDaemonProvider(tmpDirGen *file.TempDirGenerator) image.Provider {
	return docker.NewAPIClientProvider(Daemon, tmpDirGen, func() (client.APIClient, error) {
		return podman.GetClient()
	})
}
