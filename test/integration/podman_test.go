package integration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope/internal/podman"
)

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

				cmd := exec.Command("make")
				cmd.Dir = fixturesPath
				runAndShow(t, cmd)

				t.Setenv("CONTAINER_HOST", "ssh://root@localhost:2222/run/podman/podman.sock")

				keyPath := filepath.Join(fixturesPath, "ssh", "id_ed25519")
				t.Setenv("CONTAINER_SSHKEY", keyPath)

				t.Logf("ssh key %s", keyPath)

				start := time.Now()
				attempt := 1
				for {
					time.Sleep(time.Second * 2)

					t.Logf("waiting for podman to be ready (attempt %d)", attempt)
					if time.Since(start) > time.Second*30 {
						t.Fatal("timed out waiting for sshd to start")
					}
					cmd = exec.Command("make", "status")
					cmd.Dir = fixturesPath
					if err = runAndShowPassive(t, cmd); err == nil {
						t.Log("podman is ready")
						break
					}

					attempt++
				}

			},
			cleanup: func() {
				cwd, err := os.Getwd()
				assert.NoErrorf(t, err, "unable to get cwd: %+v", err)

				fixturesPath := filepath.Join(cwd, "test-fixtures", "podman")

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
