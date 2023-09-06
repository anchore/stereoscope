package containerd

import (
	"fmt"
	"github.com/adrg/xdg"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/defaults"
	"github.com/containerd/containerd/namespaces"
	"os"
)

func GetClient() (*containerd.Client, error) {
	client, err := containerd.New(Address())
	if err != nil {
		return nil, err
	}
	return client, nil
}

func Address() string {
	envAddr := os.Getenv("CONTAINERD_ADDRESS")
	if envAddr != "" {
		return envAddr
	}

	if os.Getpid() == 0 {
		return defaults.DefaultAddress
	}

	// look for rootless address (fallback to default if not found)
	//export CONTAINERD_ADDRESS=/proc/$(cat $XDG_RUNTIME_DIR/containerd-rootless/child_pid)/root/run/containerd/containerd.sock

	p := fmt.Sprintf("%s/containerd-rootless/child_pid", xdg.RuntimeDir)
	if _, err := os.Stat(p); err != nil {
		return defaults.DefaultAddress
	}

	by, err := os.ReadFile(p)
	if err != nil {
		return defaults.DefaultAddress
	}

	if len(by) == 0 {
		return defaults.DefaultAddress
	}

	return fmt.Sprintf("/proc/%s/root/run/containerd/containerd.sock", string(by))
}

func Namespace() string {
	namespace := os.Getenv("CONTAINERD_NAMESPACE")
	if namespace == "" {
		namespace = namespaces.Default
	}

	return namespace
}
