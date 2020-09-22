package image

import (
	"archive/tar"
	"github.com/spf13/afero"
	"io"
	"os"
	"path"
	"strings"
	"testing"
)

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
			source:   "docker-tar",
			expected: DockerTarballSource,
		},
		{
			source:   "docker-tarball",
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
			source:   "oci-archive",
			expected: OciTarballSource,
		},
		{
			source:   "oci-tar",
			expected: OciTarballSource,
		},
		{
			source:   "oci-tarball",
			expected: OciTarballSource,
		},
		{
			source:   "oci",
			expected: OciDirectorySource,
		},
		{
			source:   "oci-dir",
			expected: OciDirectorySource,
		},
		{
			source:   "oci-directory",
			expected: OciDirectorySource,
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

func getDummyTar(t *testing.T, fs *afero.MemMapFs, paths ...string) string {
	archivePath := "/image.tar"
	testFile, err := fs.Create(archivePath)
	if err != nil {
		t.Fatalf("failed to create dummy tar: %+v", err)
	}

	tarWriter := tar.NewWriter(testFile)
	defer tarWriter.Close()

	for _, filePath := range paths {
		header := &tar.Header{
			Name: filePath,
			Size: 13,
		}

		err = tarWriter.WriteHeader(header)
		if err != nil {
			t.Fatalf("could not write dummy header: %+v", err)
		}

		_, err = io.Copy(tarWriter, strings.NewReader("hello, world!"))
		if err != nil {
			t.Fatalf("could not write dummy file: %+v", err)
		}
	}

	return archivePath
}

func getDummyPath(t *testing.T, fs *afero.MemMapFs, paths ...string) string {
	dirPath := "/image"
	err := fs.Mkdir(dirPath, os.ModePerm)
	if err != nil {
		t.Fatalf("failed to create dummy tar: %+v", err)
	}

	for _, filePath := range paths {
		f, err := fs.Create(path.Join(dirPath, filePath))
		if err != nil {
			t.Fatalf("unable to create file: %+v", err)
		}

		if _, err = f.WriteString("hello, world!"); err != nil {
			t.Fatalf("unable to write file")
		}

		if err = f.Close(); err != nil {
			t.Fatalf("unable to close file")
		}
	}

	return dirPath
}

func TestDetectSourceFromPath(t *testing.T) {
	tests := []struct {
		name           string
		paths          []string
		expectedSource Source
		sourceType     string
	}{
		{
			name:           "no tar paths",
			paths:          []string{},
			sourceType:     "tar",
			expectedSource: UnknownSource,
		},
		{
			name:           "dummy tar paths",
			paths:          []string{"manifest", "index", "oci_layout"},
			sourceType:     "tar",
			expectedSource: UnknownSource,
		},
		{
			name:           "oci-layout tar path",
			paths:          []string{"oci-layout"},
			sourceType:     "tar",
			expectedSource: OciTarballSource,
		},
		{
			name:           "index.json tar path",
			paths:          []string{"index.json"}, // this is an optional OCI file...
			sourceType:     "tar",
			expectedSource: UnknownSource, // ...which we should not respond to as primary evidence
		},
		{
			name:           "docker tar path",
			paths:          []string{"manifest.json"},
			sourceType:     "tar",
			expectedSource: DockerTarballSource,
		},
		{
			name:           "no dir paths",
			paths:          []string{""},
			sourceType:     "dir",
			expectedSource: UnknownSource,
		},
		{
			name:           "oci-layout path",
			paths:          []string{"oci-layout"},
			sourceType:     "dir",
			expectedSource: OciDirectorySource,
		},
		{
			name:           "dummy dir paths",
			paths:          []string{"manifest", "index", "oci_layout"},
			sourceType:     "dir",
			expectedSource: UnknownSource,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fs := afero.NewMemMapFs()
			var testPath string
			switch test.sourceType {
			case "tar":
				testPath = getDummyTar(t, fs.(*afero.MemMapFs), test.paths...)
			case "dir":
				testPath = getDummyPath(t, fs.(*afero.MemMapFs), test.paths...)
			default:
				t.Fatalf("unknown source type: %+v", test.sourceType)
			}
			actual := detectSourceFromPath(fs, testPath)
			if actual != test.expectedSource {
				t.Errorf("unexpected source: %+v (expected: %+v)", actual, test.expectedSource)
			}
		})
	}
}
