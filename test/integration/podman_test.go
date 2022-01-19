package integration

import (
	"context"
	"strings"
	"testing"

	"github.com/anchore/stereoscope/internal/podman"

	"github.com/stretchr/testify/assert"
)

// test conditions:
// - assume that podman has already been setup (podman-machine)

// test cases: all test cases will use client.Ping()

// unit:
// - getPodmanAddress

// To run it locally make sure you have podman up and running:
// install:
//	$ brew install podman
// config and start:
// 	$ podman machine init
//  $ podman machine start
func TestPodmanOverSSH(t *testing.T) {
	//	if runtime.GOOS != "darwin" {
	//		t.Skip("test meant for darwin, since github actions doesn't support KVM")
	//	}

	client, err := podman.ClientOverSSH()
	assert.NoError(t, err)
	t.Logf("client version: %s", client.ClientVersion())

	p, err := client.Ping(context.Background())
	assert.NoError(t, err)
	assert.NotNil(t, p)
	t.Logf("ping: %+v", p)

	version, err := client.ServerVersion(context.Background())
	assert.NoError(t, err)
	assert.True(t, strings.HasPrefix(version.Platform.Name, "linux/amd64/fedora"))
	t.Logf("%+v", version)
}
