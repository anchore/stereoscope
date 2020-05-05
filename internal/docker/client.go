package docker

import (
	"github.com/docker/docker/client"
	"golang.org/x/net/context"
	"sync"
)

var instance *client.Client
var once sync.Once

func GetClient() *client.Client {
	once.Do(func() {
		ctx := context.Background()
		cli, err := client.NewClientWithOpts(client.FromEnv)
		if err != nil {
			panic(err)
		}
		cli.NegotiateAPIVersion(ctx)
		instance = cli
	})
	return instance
}
