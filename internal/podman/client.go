package podman

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/anchore/stereoscope/internal/log"
	"github.com/docker/docker/client"
	"github.com/pkg/errors"
)

var (
	ErrNoSocketAddress = errors.New("no socket address")
	ErrNoHostAddress   = errors.New("no host address")
)

func ClientOverSSH() (*client.Client, error) {
	var clientOpts = []client.Opt{
		client.WithAPIVersionNegotiation(),
	}

	host, identity := getSSHAddress(configPaths)

	passPhrase := ""
	if v, found := os.LookupEnv("CONTAINER_PASSPHRASE"); found {
		passPhrase = v
	}

	sshConf, err := newSSHConf(host, identity, passPhrase)
	if err != nil {
		return nil, err
	}

	log.Debugf("ssh client params: %+v", sshConf)

	httpClient, err := httpClientOverSSH(sshConf)
	if err != nil {
		return nil, fmt.Errorf("making http client: %w", err)
	}

	clientOpts = append(clientOpts, func(c *client.Client) error {
		return client.WithHTTPClient(httpClient)(c)
	})
	// This http path is defined by podman's docs: https://github.com/containers/podman/blob/main/pkg/api/server/docs.go#L31-L34
	clientOpts = append(clientOpts, client.WithHost("http://d"))

	c, err := client.NewClientWithOpts(clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed create remote client for podman: %w", err)
	}

	return c, err
}

func ClientOverUnixSocket() (*client.Client, error) {
	var clientOpts = []client.Opt{
		client.WithAPIVersionNegotiation(),
	}

	addr := getUnixSocketAddress(configPaths)
	if addr == "" {
		return nil, ErrNoSocketAddress
	}

	clientOpts = append(clientOpts, client.WithHost(addr))

	c, err := client.NewClientWithOpts(clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("creating local client for podman: %w", err)
	}

	return c, err
}

func GetClient() (*client.Client, error) {
	c, err := ClientOverUnixSocket()
	if errors.Is(err, ErrNoSocketAddress) {
		return ClientOverSSH()
	}

	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.TODO(), time.Second*3)
	defer cancel()
	_, err = c.Ping(ctx)
	return c, err
}
