package image

import (
	"archive/tar"
	"io"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/mitchellh/go-homedir"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/sylabs/sif/v2/pkg/sif"
)

func Test(t *testing.T) {
	cases := []struct {
		desc        string
		image       string
		sourceInput []Source
		expected    Source
	}{
		{
			desc:        "No input returns UnknownSource",
			image:       "test:test",
			sourceInput: []Source{},
			expected:    UnknownSource,
		},
		{
			desc:  "OciRegistry is returned when preferenced first",
			image: "test:test",
			sourceInput: []Source{
				OciRegistrySource,
				DockerDaemonSource,
				PodmanDaemonSource,
			},
			expected: OciRegistrySource,
		},
		// TODO: dependency injection update to test docker/podman cases

	}
	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			s := DetermineDefaultImagePullSource(c.image, c.sourceInput)
			assert.Equal(t, s, c.expected)
		})
	}
}

func TestDetectSource(t *testing.T) {
	cases := []struct {
		name             string
		fs               afero.Fs
		input            string
		source           Source
		expectedLocation string
	}{
		{
			name:             "podman-engine",
			input:            "podman:something:latest",
			source:           PodmanDaemonSource,
			expectedLocation: "something:latest",
		},
		{
			name:             "docker-archive",
			input:            "docker-archive:a/place.tar",
			source:           DockerTarballSource,
			expectedLocation: "a/place.tar",
		},
		{
			name:             "docker-engine-by-possible-id",
			input:            "a5e",
			source:           UnknownSource,
			expectedLocation: "",
		},
		{
			name: "docker-engine-impossible-id",
			// not a valid ID
			input:            "a5E",
			source:           UnknownSource,
			expectedLocation: "",
		},
		{
			name:             "docker-engine",
			input:            "docker:something/something:latest",
			source:           DockerDaemonSource,
			expectedLocation: "something/something:latest",
		},
		{
			name:   "docker-engine-edge-case",
			input:  "docker:latest",
			source: DockerDaemonSource,
			// we want to be able to handle this case better, however, I don't see a way to do this
			// the user will need to provide more explicit input (docker:docker:latest)
			expectedLocation: "latest",
		},
		{
			name:             "docker-engine-edge-case-explicit",
			input:            "docker:docker:latest",
			source:           DockerDaemonSource,
			expectedLocation: "docker:latest",
		},
		{
			name:             "docker-caps",
			input:            "DoCKEr:something/something:latest",
			source:           DockerDaemonSource,
			expectedLocation: "something/something:latest",
		},
		{
			name:             "infer-docker-engine",
			input:            "something/something:latest",
			source:           UnknownSource,
			expectedLocation: "",
		},
		{
			name:             "bad-hint",
			input:            "blerg:something/something:latest",
			source:           UnknownSource,
			expectedLocation: "",
		},
		{
			name:             "relative-path-1",
			input:            ".",
			source:           UnknownSource,
			expectedLocation: "",
		},
		{
			name:             "relative-path-2",
			input:            "./",
			source:           UnknownSource,
			expectedLocation: "",
		},
		{
			name:             "relative-parent-path",
			input:            "../",
			source:           UnknownSource,
			expectedLocation: "",
		},
		{
			name:             "oci-tar-path",
			fs:               getDummyTar(t, "a-potential/path", "oci-layout"),
			input:            "a-potential/path",
			source:           OciTarballSource,
			expectedLocation: "a-potential/path",
		},
		{
			name:             "unparsable-existing-path",
			fs:               getDummyTar(t, "a-potential/path"),
			input:            "a-potential/path",
			source:           UnknownSource,
			expectedLocation: "",
		},
		// honor tilde expansion
		{
			name:             "oci-tar-path",
			fs:               getDummyTar(t, "~/a-potential/path", "oci-layout"),
			input:            "~/a-potential/path",
			source:           OciTarballSource,
			expectedLocation: "~/a-potential/path",
		},
		{
			name:             "oci-tar-path-explicit",
			fs:               getDummyTar(t, "~/a-potential/path", "oci-layout"),
			input:            "oci-archive:~/a-potential/path",
			source:           OciTarballSource,
			expectedLocation: "~/a-potential/path",
		},
		{
			name:             "oci-tar-path-with-scheme-separator",
			fs:               getDummyTar(t, "a-potential/path:version", "oci-layout"),
			input:            "a-potential/path:version",
			source:           OciTarballSource,
			expectedLocation: "a-potential/path:version",
		},
		{
			name:             "singularity-path",
			fs:               getDummySIF(t, "~/a-potential/path.sif"),
			input:            "singularity:~/a-potential/path.sif",
			source:           SingularitySource,
			expectedLocation: "~/a-potential/path.sif",
		},
		{
			name:             "singularity-path-tilde",
			fs:               getDummySIF(t, "~/a-potential/path.sif"),
			input:            "~/a-potential/path.sif",
			source:           SingularitySource,
			expectedLocation: "~/a-potential/path.sif",
		},
		{
			name:             "singularity-path-explicit",
			fs:               getDummySIF(t, "~/a-potential/path.sif"),
			input:            "singularity:~/a-potential/path.sif",
			source:           SingularitySource,
			expectedLocation: "~/a-potential/path.sif",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fs := c.fs
			if c.fs == nil {
				fs = afero.NewMemMapFs()
			}

			source, location, err := detectSource(fs, c.input)
			if err != nil {
				t.Fatalf("unexecpted error: %+v", err)
			}
			if c.source != source {
				t.Errorf("expected: %q , got: %q", c.source, source)
			}

			// lean on the users real home directory value
			expandedExpectedLocation, err := homedir.Expand(c.expectedLocation)
			if err != nil {
				t.Fatalf("unable to expand path=%q: %+v", c.expectedLocation, err)
			}

			if expandedExpectedLocation != location {
				t.Errorf("expected: %q , got: %q", expandedExpectedLocation, location)
			}
		})
	}
}

func TestParseScheme(t *testing.T) {
	cases := []struct {
		source   string
		expected Source
	}{
		{
			// regression for unsupported behavior
			source:   "tar",
			expected: UnknownSource,
		},
		{
			// regression for unsupported behavior
			source:   "tarball",
			expected: UnknownSource,
		},
		{
			// regression for unsupported behavior
			source:   "archive",
			expected: UnknownSource,
		},
		{
			source:   "docker-archive",
			expected: DockerTarballSource,
		},
		{
			// regression for unsupported behavior
			source:   "docker-tar",
			expected: UnknownSource,
		},
		{
			// regression for unsupported behavior
			source:   "docker-tarball",
			expected: UnknownSource,
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
			// regression for unsupported behavior
			source:   "docker-daemon",
			expected: UnknownSource,
		},
		{
			// regression for unsupported behavior
			source:   "docker-engine",
			expected: UnknownSource,
		},
		{
			source:   "oci-archive",
			expected: OciTarballSource,
		},
		{
			// regression for unsupported behavior
			source:   "oci-tar",
			expected: UnknownSource,
		},
		{
			// regression for unsupported behavior
			source:   "oci-tarball",
			expected: UnknownSource,
		},
		{
			// regression for unsupported behavior
			source:   "oci",
			expected: UnknownSource,
		},
		{
			source:   "oci-dir",
			expected: OciDirectorySource,
		},
		{
			// regression for unsupported behavior
			source:   "oci-directory",
			expected: UnknownSource,
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
		actual := ParseSourceScheme(c.source)
		if c.expected != actual {
			t.Errorf("unexpected source: %s!=%s", c.expected, actual)
		}
	}
}

func TestDetectSourceFromPath(t *testing.T) {
	tests := []struct {
		name           string
		path           string
		fs             afero.Fs
		expectedSource Source
		expectedErr    bool
	}{
		{
			name:           "no tar paths",
			path:           "image.tar",
			fs:             getDummyTar(t, "image.tar"),
			expectedSource: UnknownSource,
		},
		{
			name:           "dummy tar paths",
			path:           "image.tar",
			fs:             getDummyTar(t, "image.tar", "manifest", "index", "oci_layout"),
			expectedSource: UnknownSource,
		},
		{
			name:           "oci-layout tar path",
			path:           "image.tar",
			fs:             getDummyTar(t, "image.tar", "oci-layout"),
			expectedSource: OciTarballSource,
		},
		{
			name:           "index.json tar path",
			path:           "image.tar",
			fs:             getDummyTar(t, "image.tar", "index.json"), // this is an optional OCI file...
			expectedSource: UnknownSource,                             // ...which we should not respond to as primary evidence
		},
		{
			name:           "docker tar path",
			path:           "image.tar",
			fs:             getDummyTar(t, "image.tar", "manifest.json"),
			expectedSource: DockerTarballSource,
		},
		{
			name:           "no dir paths",
			path:           "image",
			fs:             getDummyDir(t, "image"),
			expectedSource: UnknownSource,
		},
		{
			name:           "oci-layout path",
			path:           "image",
			fs:             getDummyDir(t, "image", "oci-layout"),
			expectedSource: OciDirectorySource,
		},
		{
			name:           "dummy dir paths",
			path:           "image",
			fs:             getDummyDir(t, "image", "manifest", "index", "oci_layout"),
			expectedSource: UnknownSource,
		},
		{
			name:           "no path given",
			path:           "/does-not-exist",
			expectedSource: UnknownSource,
			expectedErr:    false,
		},
		{
			name:           "singularity path",
			path:           "image.sif",
			fs:             getDummySIF(t, "image.sif"),
			expectedSource: SingularitySource,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fs := test.fs
			if fs == nil {
				fs = afero.NewMemMapFs()
			}

			actual, err := detectSourceFromPath(fs, test.path)
			if err != nil && !test.expectedErr {
				t.Fatalf("unexpected error: %+v", err)
			} else if err == nil && test.expectedErr {
				t.Fatal("expected error but got none")
			}
			if actual != test.expectedSource {
				t.Errorf("unexpected source: %+v (expected: %+v)", actual, test.expectedSource)
			}
		})
	}
}

// getDummyTar returns a filesystem that contains a TAR archive at archivePath populated with paths.
func getDummyTar(t *testing.T, archivePath string, paths ...string) afero.Fs {
	t.Helper()

	archivePath, err := homedir.Expand(archivePath)
	if err != nil {
		t.Fatalf("unable to expand home path=%q: %+v", archivePath, err)
	}

	fs := afero.NewMemMapFs()

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

	return fs
}

// getDummyDir returns a filesystem that contains directory dirPath populated with paths.
func getDummyDir(t *testing.T, dirPath string, paths ...string) afero.Fs {
	t.Helper()

	dirPath, err := homedir.Expand(dirPath)
	if err != nil {
		t.Fatalf("unable to expand home dir=%q: %+v", dirPath, err)
	}

	fs := afero.NewMemMapFs()

	if err = fs.Mkdir(dirPath, os.ModePerm); err != nil {
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

	return fs
}

// getDummySIF returns a filesystem that contains a SIF at path.
func getDummySIF(t *testing.T, path string, opts ...sif.CreateOpt) afero.Fs {
	t.Helper()

	path, err := homedir.Expand(path)
	if err != nil {
		t.Fatalf("unable to expand home dir=%q: %+v", path, err)
	}

	fs := afero.NewMemMapFs()

	f, err := fs.Create(path)
	if err != nil {
		t.Fatalf("failed to create file: %+v", err)
	}
	defer f.Close()

	fi, err := sif.CreateContainer(f, opts...)
	if err != nil {
		t.Fatalf("failed to create container: %+v", err)
	}
	defer fi.UnloadContainer()

	return fs
}
