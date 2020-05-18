// +build integration

package integration

import (
	"testing"
)


func TestSimpleImageFiletrees(t *testing.T) {
	fixtureName := "image-simple"
	cases := []struct{
		name string
		source string
		fixtureName string
	} {
		{
			name: "FromTarball",
			source: "tarball",
			fixtureName: fixtureName,
		},
		{
			name: "FromDocker",
			source: "docker",
			fixtureName: fixtureName,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T){
			i := getSquashedImage(t, c.source, c.fixtureName)
			assertImageSimpleFixtureTrees(t, i)
		})
	}

}

func TestSimpleImageMultipleFileContents(t *testing.T) {
	fixtureName := "image-simple"
	cases := []struct{
		name string
		source string
		fixtureName string
	} {
		{
			name: "FromTarball",
			source: "tarball",
			fixtureName: fixtureName,
		},
		{
			name: "FromDocker",
			source: "docker",
			fixtureName: fixtureName,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T){
			i := getSquashedImage(t, c.source, c.fixtureName)
			assertImageSimpleFixtureContents(t, i)
		})
	}

}