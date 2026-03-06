package docker

import (
	"os"
	"strings"
	"testing"

	"github.com/moby/moby/client"
)

func Test_newClient(t *testing.T) {
	cases := []struct {
		name           string
		providedSocket string
		expectedSocket string
		setEnv         func(t *testing.T)
	}{
		{
			name:           "Test newClient returns the correct default location",
			providedSocket: "",
			expectedSocket: "unix:///var/run/docker.sock",
		},
		{
			name:           "Test newClient with runtime specific path",
			providedSocket: "",
			setEnv: func(t *testing.T) {
				os.Setenv("DOCKER_HOST", "unix:///var/CUSTOM/docker.sock")

			},
			expectedSocket: "unix:///var/CUSTOM/docker.sock",
		},
		{
			name:           "Test newClient with runtime specific path",
			providedSocket: "unix:///var/NEWCUSTOM/docker.sock",
			expectedSocket: "unix:///var/NEWCUSTOM/docker.sock",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.setEnv != nil {
				c.setEnv(t)
			}
			clientOpts := []client.Opt{
				client.FromEnv,
			}

			apiClient, err := newClient(c.providedSocket, clientOpts...)
			if err != nil {
				t.Errorf("newClient() error = %v", err)
				return
			}

			if apiClient.DaemonHost() != c.expectedSocket {
				t.Errorf("newClient() = %v, want %v", apiClient.DaemonHost(), c.expectedSocket)
			}
		})
	}
}

func Test_possibleSocketPaths(t *testing.T) {
	cases := []struct {
		name     string
		provided string
		expected []string
	}{
		{
			name:     "Test possibleSocketPaths returns the correct default location for darwin",
			provided: "darwin",
			expected: []string{"", "Library/Containers/com.docker.docker/Data/docker.raw.sock"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for i, socketPath := range possibleSocketPaths(c.provided) {
				if !strings.HasSuffix(socketPath, c.expected[i]) {
					t.Errorf("possibleSocketPaths() = %v, want %v", socketPath, c.expected[i])
				}
			}
		})
	}
}
