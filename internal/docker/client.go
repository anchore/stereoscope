package docker

import (
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/anchore/stereoscope/internal/log"

	"github.com/docker/cli/cli/connhelper"
	"github.com/docker/docker/client"
)

var (
	once        sync.Once
	instance    *client.Client
	instanceErr error
)

func GetClient(srcName string) (*client.Client, error) {
	if srcName == "PodmanDaemon" {
		return getPodmanClient()
	}

	once.Do(func() {
		var clientOpts = []client.Opt{
			client.FromEnv,
			client.WithAPIVersionNegotiation(),
		}

		host := os.Getenv("DOCKER_HOST")

		if strings.HasPrefix(host, "ssh") {
			var helper *connhelper.ConnectionHelper
			var err error

			helper, err = connhelper.GetConnectionHelper(host)

			if err != nil {
				log.Errorf("failed to fetch docker connection helper: %w", err)
				instanceErr = err
				return
			}
			clientOpts = append(clientOpts, func(c *client.Client) error {
				httpClient := &http.Client{
					Transport: &http.Transport{
						DialContext: helper.Dialer,
					},
				}
				return client.WithHTTPClient(httpClient)(c)
			})
			clientOpts = append(clientOpts, client.WithHost(helper.Host))
			clientOpts = append(clientOpts, client.WithDialContext(helper.Dialer))
		}

		if os.Getenv("DOCKER_TLS_VERIFY") != "" && os.Getenv("DOCKER_CERT_PATH") == "" {
			err := os.Setenv("DOCKER_CERT_PATH", "~/.docker")
			if err != nil {
				log.Errorf("failed create docker client: %w", err)
				instanceErr = err
				return
			}
		}
		dockerClient, err := client.NewClientWithOpts(clientOpts...)
		if err != nil {
			log.Errorf("failed create docker client: %w", err)
			instanceErr = err
			return
		}

		instance = dockerClient
	})

	return instance, instanceErr
}
