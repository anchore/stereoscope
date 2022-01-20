package integration

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/anchore/stereoscope/internal/podman"
	"github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		setup       func(*testing.T)
	}{
		{
			name:        "ssh connection",
			constructor: podman.ClientOverSSH,
			setup:       setupSSHKeys,
		},
		{
			name:        "unix socket connection",
			constructor: podman.ClientOverUnixSocket,
			setup:       func(t *testing.T) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup(t)
			c, err := tt.constructor()
			assert.NoError(t, err)
			assert.NotEmpty(t, c.ClientVersion())

			p, err := c.Ping(context.Background())
			assert.NoError(t, err)
			assert.NotNil(t, p)

			version, err := c.ServerVersion(context.Background())
			assert.NoError(t, err)
			assert.NotEmpty(t, version)
		})
	}
}

func setupSSHKeys(t *testing.T) {
	require.Equalf(t, "linux", runtime.GOOS, "setup meant for CI -- it can modify your ssh authorized keys")

	addr := fmt.Sprintf("ssh://localhost/run/user/%d/podman/podman.sock", os.Getuid())
	err := os.Setenv("CONTAINER_HOST", addr)
	assert.NoError(t, err)
	// ssh-keygen -t rsa -f test-rsa -N "passphrase"
	keyFile := filepath.Join(os.TempDir(), "integration-test-key")

	err = os.Setenv("CONTAINER_SSHKEY", keyFile)
	assert.NoError(t, err)

	// 0: making key
	out, err := exec.Command("ssh-keygen", "-t", "rsa", "-f", keyFile, "-N", "").Output()
	require.NoError(t, err)
	t.Logf("output: %s", out)

	// 1: append to authorized_keys
	b, err := ioutil.ReadFile(keyFile + ".pub")
	require.NoError(t, err)

	// If the file doesn't exist, create it, or append to the file
	f, err := os.OpenFile("~/.ssh/authorized_keys", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	require.NoError(t, err)
	size, err := f.Write(b)
	require.NoError(t, err)
	assert.NotZero(t, size)
}
