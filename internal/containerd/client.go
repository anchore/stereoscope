package containerd

import (
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/defaults"
)

var (
	defaultAddr = defaults.DefaultAddress
)

func GetClient(addr string) (*containerd.Client, error) {
	if addr == "" {
		addr = defaultAddr
	}

	client, err := containerd.New(addr)
	if err != nil {
		return nil, err
	}
	return client, nil
}
