package integration

import (
	"bufio"
	"context"
	"github.com/anchore/stereoscope/internal/podman"
	"github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func runAndShow(t *testing.T, cmd *exec.Cmd) {
	t.Helper()

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
}

// This was commented out until we can confirm the new behavior of the github runner
// tests started throwing "read: connection reset by peer" when connecting to ssh://root@localhost:2222/run/podman/podman.sock
// we might need to think of another creative way to test this, but in the meantime it has been failing stereoscope builds

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
				runAndShow(t, cmd)

				err = os.Setenv("CONTAINER_HOST", "ssh://root@localhost:2222/run/podman/podman.sock")
				assert.NoError(t, err)

				keyPath := filepath.Join(fixturesPath, "ssh", "id_ed25519")
				err = os.Setenv("CONTAINER_SSHKEY", keyPath)
				assert.NoError(t, err)
				t.Logf("ssh key %s", keyPath)

				time.Sleep(time.Second) // TODO: sync so test starts when docker is ready
			},
			cleanup: func() {
				cwd, err := os.Getwd()
				assert.NoErrorf(t, err, "unable to get cwd: %+v", err)
				err = os.Unsetenv("CONTAINER_HOST")
				assert.NoError(t, err)
				err = os.Unsetenv("CONTAINER_SSHKEY")
				assert.NoError(t, err)

				// TODO stop podman-ssh
				fixturesPath := filepath.Join(cwd, "test-fixtures", "podman")
				makeTask := filepath.Join(fixturesPath, "Makefile")
				t.Logf("Generating Fixture from 'make %s'", makeTask)

				cmd := exec.Command("make", "stop")
				cmd.Dir = fixturesPath

				runAndShow(t, cmd)
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
			require.NoError(t, err)
			assert.NotEmpty(t, c.ClientVersion())

			p, err := c.Ping(context.Background())
			require.NoError(t, err)
			assert.NotNil(t, p)

			version, err := c.ServerVersion(context.Background())
			require.NoError(t, err)
			assert.NotEmpty(t, version)
		})
	}
}
