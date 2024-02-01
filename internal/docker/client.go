package docker

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/docker/cli/cli/connhelper"
	"github.com/docker/docker/client"
	"github.com/mitchellh/go-homedir"
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
	// If it fails, it tries to create a client with the runtime specific path
	dockerClient, err := newClient("", clientOpts...)
	if err == nil {
		err := checkConnection(dockerClient)
		if err == nil {
			return dockerClient, nil // Successfully connected
		}
	}

	// If the client socket didn't work, try the GOOS specific path
	// This is useful for macOS, where the default Unix socket for newer distributions of docker desktop is different
	dockerClient, err = newClient(runtime.GOOS, clientOpts...)
	if err == nil {
		err := checkConnection(dockerClient)
		if err == nil {
			return dockerClient, nil // Successfully connected
		}
	}

	// If both attempts failed
	return nil, fmt.Errorf("failed to connect to Docker daemon. Ensure Docker is running and accessible")
}

func newClient(os string, opts ...client.Opt) (*client.Client, error) {
	switch os {
	case "darwin":
		hDir, err := homedir.Dir()
		if err != nil {
			return nil, err
		}
		macOSSocketPath := filepath.Join(hDir, "Library/Containers/com.docker.docker/Data/docker.raw.sock")
		opts = append(opts, client.WithHost("unix://"+macOSSocketPath))
	default:
	}
	return client.NewClientWithOpts(opts...)
}

func checkConnection(client *client.Client) error {
	_, err := client.Ping(context.Background())
	return err
}
