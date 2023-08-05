package docker

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"

	"github.com/anchore/stereoscope/internal/log"
	"github.com/docker/cli/cli/connhelper"
	"github.com/docker/docker/client"
)

func GetClient() (*client.Client, error) {
	_, found := os.LookupEnv("DOCKER_HOST")
	if !found {

		log.Debugf("no explicit DOCKER_HOST defined")

		log.Debugf("reading docker configuration: %s", configFileName)

		cfg, err := loadConfig(configFileName)
		if err != nil && errors.Is(err, fs.ErrNotExist) {
			log.Debugf("no docker configuration found", configFileName)

			err = nil
		}

		if err != nil {
			return nil, fmt.Errorf("cant parse docker config: %w", err)
		}

		if cfg != nil {
			dockerContext := resolveContextName(cfg)

			log.Debugf("current docker context: %s", dockerContext)

			host, err := endpointFromContext(dockerContext)
			if err != nil {
				return nil, err
			}

			log.Debugf("using host from docker configuration: %s", host)

			err = os.Setenv("DOCKER_HOST", host)
			if err != nil {
				return nil, err
			}

		}
	}

	clientOpts := []client.Opt{
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	}

	host := os.Getenv("DOCKER_HOST")

	if strings.HasPrefix(host, "ssh") {
		var (
			helper *connhelper.ConnectionHelper
			err    error
		)

		helper, err = connhelper.GetConnectionHelper(host)

		if err != nil {
			return nil, fmt.Errorf("failed to fetch docker connection helper: %w", err)
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
			return nil, fmt.Errorf("failed create docker client: %w", err)
		}
	}

	dockerClient, err := client.NewClientWithOpts(clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed create docker client: %w", err)
	}

	return dockerClient, nil
}
