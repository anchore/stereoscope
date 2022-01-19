package integration

import (
	"context"
	"testing"

	"github.com/anchore/stereoscope/internal/podman"

	"github.com/stretchr/testify/assert"
)

// test conditions:
// - assume that podman has already been setup (podman-machine)

// test cases: all test cases will use client.Ping()

// unit:
// - getPodmanAddress

func TestPodmanOverSSH(t *testing.T) {
	client, err := podman.ClientOverSSH()
	assert.NoError(t, err)
	t.Logf("client version: %s", client.ClientVersion())

	p, err := client.Ping(context.Background())
	assert.NoError(t, err)
	assert.NotNil(t, p)
	t.Logf("ping: %+v", p)
}
