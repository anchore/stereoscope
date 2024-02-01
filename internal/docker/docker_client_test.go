package docker

import (
	"strings"
	"testing"
)

func Test_newClient(t *testing.T) {
	cases := []struct {
		name           string
		osCase         string
		expectedSocket string
	}{
		{
			name:           "Test newClient with default options",
			osCase:         "",
			expectedSocket: "unix:///var/run/docker.sock",
		},
		{
			name:           "Test newClient with runtime specific path",
			osCase:         "darwin",
			expectedSocket: "Library/Containers/com.docker.docker/Data/docker.raw.sock",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dockerClient, err := newClient(c.osCase)
			if err != nil {
				t.Errorf("Error: %v", err)
			}
			if dockerClient == nil {
				t.Errorf("Error: dockerClient is nil")
			}
			daemonHost := dockerClient.DaemonHost()
			if daemonHost == "" {
				t.Errorf("Error: daemonHost is empty")
			}
			if strings.HasSuffix(daemonHost, c.expectedSocket) == false {
				t.Errorf("Error: daemonHost is not  contain expectedSocket: %s:%s", daemonHost, c.expectedSocket)
			}
		})
	}
}
