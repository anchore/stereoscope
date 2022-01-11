package docker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetPodmanAddress(t *testing.T) {
	addr, err := getAddressFromConfig("./test-fixtures/containers.conf")
	assert.NoError(t, err)
	assert.Equal(t, "ssh://core@localhost:63753/run/user/1000/podman/podman.sock", addr)
}
