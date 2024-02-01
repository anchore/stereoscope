package docker

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/docker/cli/cli/connhelper"
	"github.com/docker/docker/client"
)

func GetClient() (*client.Client, error) {
	var clientOpts = []client.Opt{
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

	// This tries to create a docker client with the default options
	dockerClient, err := client.NewClientWithOpts(clientOpts...)
	if err == nil {
		_, err = dockerClient.Ping(context.Background())
		if err == nil {
			return dockerClient, nil // Successfully connected
		}
	}

	// If on macOS and the default Unix socket didn't work, try the macOS-specific path
	if runtime.GOOS == "darwin" {
		user, err := user.Current()
		if err != nil {
			return nil, fmt.Errorf("failed to get current user: %w", err)
		}

		macOSSocketPath := filepath.Join(user.HomeDir, "Library/Containers/com.docker.docker/Data/docker.raw.sock")
		clientOpts = append(clientOpts, client.WithHost("unix://"+macOSSocketPath))
		dockerClient, err = client.NewClientWithOpts(clientOpts...)
		if err == nil {
			_, err := dockerClient.Ping(context.Background())
			if err == nil {
				return dockerClient, nil // Successfully connected
			}
		}

	}

	// If both attempts failed
	return nil, fmt.Errorf("failed to connect to Docker daemon. Ensure Docker is running and accessible")
}
