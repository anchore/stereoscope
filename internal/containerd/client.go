package containerd

import (
	"os"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/defaults"
	"github.com/containerd/containerd/namespaces"
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

func Namespace() string {
	namespace := os.Getenv("CONTAINERD_NAMESPACE")
	if namespace == "" {
		namespace = namespaces.Default
	}

	return namespace
}
