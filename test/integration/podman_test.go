package integration

import (
	"context"
	"testing"

	"github.com/anchore/stereoscope/internal/podman"
	"github.com/docker/docker/client"

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
func TestPodmanConnections(t *testing.T) {
	tests := []struct {
		name        string
		constructor func() (*client.Client, error)
	}{
		{
			name:        "ssh connection",
			constructor: podman.ClientOverSSH,
		},
		{
			name:        "unix socket connection",
			constructor: podman.ClientOverUnixSocket,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := tt.constructor()
			assert.NoError(t, err)
			assert.NotEmpty(t, client.ClientVersion())

			p, err := client.Ping(context.Background())
			assert.NoError(t, err)
			assert.NotNil(t, p)

			version, err := client.ServerVersion(context.Background())
			assert.NoError(t, err)
			assert.NotEmpty(t, version)
		})
	}
}
