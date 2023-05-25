package docker

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

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

	clientOpts = append(clientOpts, func(c *client.Client) error {
		httpClient := &http.Client{
			Transport: &http.Transport{
				DisableKeepAlives: true,
				IdleConnTimeout:   10 * time.Second,
			},
		}
		return client.WithHTTPClient(httpClient)(c)
	})

	clientOpts = append(clientOpts, client.WithDialContext(func(ctx context.Context, network, addr string) (net.Conn, error) {
		return net.Dial("unix", "/var/run/docker.sock") // In some cases, the path of unix socket is not /var/run/docker.sock
	}))

	dockerClient, err := client.NewClientWithOpts(clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed create docker client: %w", err)
	}

	return dockerClient, nil
}
