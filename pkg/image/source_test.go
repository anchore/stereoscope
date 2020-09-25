package image

import (
	"archive/tar"
	"github.com/mitchellh/go-homedir"
	"github.com/spf13/afero"
	"io"
	"os"
	"path"
	"strings"
	"testing"
)

func TestDetectSource(t *testing.T) {
	cases := []struct {
		name             string
		input            string
		source           Source
		expectedLocation string
		tarPath          string
		tarPaths         []string
	}{
		{
			name:             "docker-archive",
			input:            "docker-archive:a/place.tar",
			source:           DockerTarballSource,
			expectedLocation: "a/place.tar",
		},
		{
			name:             "docker-engine-by-possible-id",
			input:            "a5e",
			source:           DockerDaemonSource,
			expectedLocation: "a5e",
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
			source:           DockerDaemonSource,
			expectedLocation: "something/something:latest",
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
			input:            "a-potential/path",
			source:           OciTarballSource,
			expectedLocation: "a-potential/path",
			tarPath:          "a-potential/path",
			tarPaths:         []string{"oci-layout"},
		},
		{
			name:             "unparsable-existing-path",
			input:            "a-potential/path",
			source:           DockerDaemonSource,
			expectedLocation: "a-potential/path",
			tarPath:          "a-potential/path",
			tarPaths:         []string{},
		},
		// honor tilde expansion
		{
			name:             "oci-tar-path",
			input:            "~/a-potential/path",
			source:           OciTarballSource,
			expectedLocation: "~/a-potential/path",
			tarPath:          "~/a-potential/path",
			tarPaths:         []string{"oci-layout"},
		},
		{
			name:             "oci-tar-path-explicit",
			input:            "oci-archive:~/a-potential/path",
			source:           OciTarballSource,
			expectedLocation: "~/a-potential/path",
			tarPath:          "~/a-potential/path",
			tarPaths:         []string{"oci-layout"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fs := afero.NewMemMapFs()
			if c.tarPath != "" {
				getDummyTar(t, fs.(*afero.MemMapFs), c.tarPath, c.tarPaths...)
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
		paths          []string
		expectedSource Source
		sourceType     string
		expectedErr    bool
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
			paths:          []string{},
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
		{
			name:           "no path given",
			sourceType:     "none",
			expectedSource: UnknownSource,
			expectedErr:    false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fs := afero.NewMemMapFs()
			var testPath string
			switch test.sourceType {
			case "tar":
				testPath = getDummyTar(t, fs.(*afero.MemMapFs), "image.tar", test.paths...)
			case "dir":
				testPath = getDummyPath(t, fs.(*afero.MemMapFs), "image", test.paths...)
			case "none":
				testPath = "/does-not-exist"
			default:
				t.Fatalf("unknown source type: %+v", test.sourceType)
			}
			actual, err := detectSourceFromPath(fs, testPath)
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

// note: we do not pass the afero.Fs interface since we are writing out to the root of the filesystem, something we never want to do with an OS filesystem. This type is more explicit.
func getDummyTar(t *testing.T, fs *afero.MemMapFs, archivePath string, paths ...string) string {
	t.Helper()

	archivePath, err := homedir.Expand(archivePath)
	if err != nil {
		t.Fatalf("unable to expand home path=%q: %+v", archivePath, err)
	}

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

// note: we do not pass the afero.Fs interface since we are writing out to the root of the filesystem, something we never want to do with an OS filesystem. This type is more explicit.
func getDummyPath(t *testing.T, fs *afero.MemMapFs, dirPath string, paths ...string) string {
	t.Helper()

	dirPath, err := homedir.Expand(dirPath)
	if err != nil {
		t.Fatalf("unable to expand home dir=%q: %+v", dirPath, err)
	}

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

	return dirPath
}
