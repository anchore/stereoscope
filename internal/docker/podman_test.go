package docker

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPodmanAddress(t *testing.T) {
	{
		err := os.Setenv("XDG_RUNTIME_DIR", "/run/user/1234")
		require.NoError(t, err)
		defer os.Unsetenv("XDG_RUNTIME_DIR")

		addr, err := getPodmanAddress()
		assert.NoError(t, err)
		assert.Equal(t, "unix:///run/user/1234/podman/podman.sock", addr)
	}

	{
		addr, err := getAddressFromConfig("./test-fixtures/containers.conf")
		assert.NoError(t, err)
		assert.Equal(t, "ssh://core@localhost:63753/run/user/1000/podman/podman.sock", addr)
	}
}
