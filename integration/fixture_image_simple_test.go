// +build integration

package integration

import (
	"testing"

	"github.com/anchore/go-testutils"
)

func TestSimpleImageMetadata(t *testing.T) {
	fixtureName := "image-simple"
	cases := []struct {
		name        string
		source      string
		fixtureName string
	}{
		{
			name:        "FromTarball",
			source:      "docker-archive",
			fixtureName: fixtureName,
		},
		{
			name:        "FromDocker",
			source:      "docker",
			fixtureName: fixtureName,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			i, cleanup := testutils.GetFixtureImage(t, c.source, c.fixtureName)
			defer cleanup()
			assertImageSimpleFixtureMetadata(t, i)
		})
	}

}

func TestSimpleImageFiletrees(t *testing.T) {
	fixtureName := "image-simple"
	cases := []struct {
		name        string
		source      string
		fixtureName string
	}{
		{
			name:        "FromTarball",
			source:      "docker-archive",
			fixtureName: fixtureName,
		},
		{
			name:        "FromDocker",
			source:      "docker",
			fixtureName: fixtureName,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			i, cleanup := testutils.GetFixtureImage(t, c.source, c.fixtureName)
			defer cleanup()
			assertImageSimpleFixtureTrees(t, i)
		})
	}

}

func TestSimpleImageMultipleFileContents(t *testing.T) {
	fixtureName := "image-simple"
	cases := []struct {
		name        string
		source      string
		fixtureName string
	}{
		{
			name:        "FromTarball",
			source:      "docker-archive",
			fixtureName: fixtureName,
		},
		{
			name:        "FromDocker",
			source:      "docker",
			fixtureName: fixtureName,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			i, cleanup := testutils.GetFixtureImage(t, c.source, c.fixtureName)
			defer cleanup()
			assertImageSimpleFixtureContents(t, i)
		})
	}

}
