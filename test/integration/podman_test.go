package integration

import (
	"bufio"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

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
		cleanup     func()
	}{
		{
			name:        "ssh connection",
			constructor: podman.ClientOverSSH,
			setup: func(t *testing.T) {
				cwd, err := os.Getwd()
				require.NoErrorf(t, err, "unable to get cwd: %+v", err)

				fixturesPath := filepath.Join(cwd, "test-fixtures", "podman")
				makeTask := filepath.Join(fixturesPath, "Makefile")
				t.Logf("Generating Fixture from 'make %s'", makeTask)

				cmd := exec.Command("make")
				cmd.Dir = fixturesPath

				stderr, err := cmd.StderrPipe()
				require.NoErrorf(t, err, "could not get stderr: +v", err)

				stdout, err := cmd.StdoutPipe()
				require.NoErrorf(t, err, "could not get stdout: +v", err)

				err = cmd.Start()
				require.NoErrorf(t, err, "failed to start cmd: %+v", err)

				show := func(label string, reader io.ReadCloser) {
					scanner := bufio.NewScanner(reader)
					scanner.Split(bufio.ScanLines)
					for scanner.Scan() {
						t.Logf("%s: %s", label, scanner.Text())
					}
				}
				show("out", stdout)
				show("err", stderr)

				err = os.Setenv("CONTAINER_HOST", "ssh://root@localhost:2222/run/podman/podman.sock")
				assert.NoError(t, err)

				keyPath := filepath.Join(fixturesPath, "ssh", "id_ed25519")
				err = os.Setenv("CONTAINER_SSHKEY", keyPath)
				assert.NoError(t, err)
				t.Logf("ssh key %s", keyPath)

				time.Sleep(time.Second) // TODO: sync so test starts when docker is ready
			},
			cleanup: func() {
				err := os.Unsetenv("CONTAINER_HOST")
				assert.NoError(t, err)
				err = os.Unsetenv("CONTAINER_SSHKEY")
				assert.NoError(t, err)
			},
		},
		{
			name:        "unix socket connection",
			constructor: podman.ClientOverUnixSocket,
			setup:       func(t *testing.T) {},
			cleanup:     func() {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(tt.cleanup)

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
