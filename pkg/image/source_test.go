package image

import "testing"

func TestParseImageSpec(t *testing.T) {
	cases := []struct {
		name     string
		source   Source
		location string
	}{
		{
			name:     "tar://a/place.tar",
			source:   DockerTarballSource,
			location: "a/place.tar",
		},
		{
			name:     "docker://something/something:latest",
			source:   DockerDaemonSource,
			location: "something/something:latest",
		},
		{
			name:     "DoCKEr://something/something:latest",
			source:   DockerDaemonSource,
			location: "something/something:latest",
		},
		{
			name:     "something/something:latest",
			source:   DockerDaemonSource,
			location: "something/something:latest",
		},
		{
			name:     "blerg://something/something:latest",
			source:   UnknownSource,
			location: "",
		},
	}
	for _, c := range cases {
		source, location := ParseImageSpec(c.name)
		if c.source != source {
			t.Errorf("unexpected source: %s!=%s", c.source, source)
		}
		if c.location != location {
			t.Errorf("unexpected location: %s!=%s", c.location, location)
		}
	}
}

func TestParseSource(t *testing.T) {
	cases := []struct {
		source   string
		expected Source
	}{
		{
			source:   "tar",
			expected: DockerTarballSource,
		},
		{
			source:   "tarball",
			expected: DockerTarballSource,
		},
		{
			source:   "archive",
			expected: DockerTarballSource,
		},
		{
			source:   "docker-archive",
			expected: DockerTarballSource,
		},
		{
			source:   "Docker",
			expected: DockerDaemonSource,
		},
		{
			source:   "DOCKER",
			expected: DockerDaemonSource,
		},
		{
			source:   "docker",
			expected: DockerDaemonSource,
		},
		{
			source:   "docker-daemon",
			expected: DockerDaemonSource,
		},
		{
			source:   "docker-engine",
			expected: DockerDaemonSource,
		},
		{
			source:   "",
			expected: UnknownSource,
		},
		{
			source:   "something",
			expected: UnknownSource,
		},
	}
	for _, c := range cases {
		actual := ParseSource(c.source)
		if c.expected != actual {
			t.Errorf("unexpected source: %s!=%s", c.expected, actual)
		}
	}
}
