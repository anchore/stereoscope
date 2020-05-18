package image

import "testing"

func TestParseImageSpec(t *testing.T) {
	cases := []struct{
		name     string
		source   Source
		location string
	} {
		{
			name: "tar://a/place.tar",
			source: TarballSource,
			location: "a/place.tar",
		},
		{
			name: "docker://something/something:latest",
			source: DockerSource,
			location: "something/something:latest",
		},
		{
			name: "DoCKEr://something/something:latest",
			source: DockerSource,
			location: "something/something:latest",
		},
		{
			name: "something/something:latest",
			source: DockerSource,
			location: "something/something:latest",
		},
		{
			name: "blerg://something/something:latest",
			source: UnknownSource,
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
	cases := []struct{
		source string
		expected Source
	} {
		{
			source: "tar",
			expected: TarballSource,
		},
		{
			source: "tarball",
			expected: TarballSource,
		},
		{
			source: "archive",
			expected: TarballSource,
		},
		{
			source: "docker-archive",
			expected: TarballSource,
		},
		{
			source: "Docker",
			expected: DockerSource,
		},
		{
			source: "DOCKER",
			expected: DockerSource,
		},
		{
			source: "docker",
			expected: DockerSource,
		},
		{
			source: "docker-daemon",
			expected: DockerSource,
		},
		{
			source: "docker-engine",
			expected: DockerSource,
		},
		{
			source: "",
			expected: UnknownSource,
		},
		{
			source: "something",
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