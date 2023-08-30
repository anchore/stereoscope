package containerd

import (
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/defaults"
	"os"
)

func GetClient() (*containerd.Client, error) {
	addr := defaults.DefaultAddress
	envAddr := os.Getenv("CONTAINERD_ADDRESS")
	if envAddr != "" {
		addr = envAddr
	}

	client, err := containerd.New(addr)
	if err != nil {
		return nil, err
	}
	return client, nil
}
