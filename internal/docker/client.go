package docker

import (
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/docker/cli/cli/connhelper"

	"github.com/docker/docker/client"
)

var instance *client.Client
var once sync.Once

func GetClient() *client.Client {
	once.Do(func() {
		var clientOpts = []client.Opt{
			client.FromEnv,
			client.WithAPIVersionNegotiation(),
		}

		host := os.Getenv("DOCKER_HOST")

		if strings.HasPrefix(host, "ssh") {
			helper, err := connhelper.GetConnectionHelper(host)
			if err != nil {
				// TODO: replace
				panic(err)
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
			_ = os.Setenv("DOCKER_CERT_PATH", "~/.docker")
			// TODO: log
			//if err != nil {
			//
			//}
		}
		dockerClient, err := client.NewClientWithOpts(clientOpts...)
		if err != nil {
			// TODO: replace
			panic(err)
		}

		instance = dockerClient
	})
	return instance
}
