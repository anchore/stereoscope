// +build integration

package integration

import (
	"crypto/sha256"
	"fmt"
	"github.com/anchore/stereoscope"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/image"
	"github.com/anchore/stereoscope/pkg/tree"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"testing"
)

const (
	testFixturesDirName = "test-fixtures"
	tarCacheDirName     = "tar-cache"
	imagePrefix         = "stereoscope-fixture"
)

func compareLayerTrees(t *testing.T, expected map[uint]*tree.FileTree, i *image.Image, ignorePaths []file.Path) {
	t.Helper()
	if len(expected) != len(i.Layers) {
		t.Fatalf("mismatched layers (%d!=%d)", len(expected), len(i.Layers))
	}

	for idx, expected := range expected {
		actual := i.Layers[idx].Tree
		if !expected.Equal(actual) {
			extra, missing := expected.PathDiff(actual)
			nonIgnoredPaths := 0

			for _, p := range extra {
				found := false
				inner1: for _, ignore := range ignorePaths {
					if ignore == p {
						found = true
						break inner1
					}
				}
				if !found {
					nonIgnoredPaths++
				}
			}

			for _, p := range missing {
				found := false
				inner2: for _, ignore := range ignorePaths {
				if ignore == p {
					found = true
					break inner2
				}
			}
				if !found {
					nonIgnoredPaths++
				}
			}
			if nonIgnoredPaths > 0 {
				t.Logf("ignore paths: %+v", ignorePaths)
				t.Logf("path differences: extra=%+v missing=%+v", extra, missing)
				t.Errorf("mismatched trees (layer %d)", idx)
			}
		}
	}
}

func compareSquashTree(t *testing.T, expected *tree.FileTree, i *image.Image) {
	t.Helper()

	actual := i.SquashedTree
	if !expected.Equal(actual) {
		fmt.Println(expected.PathDiff(actual))
		t.Errorf("mismatched squashed trees")
	}

}

func getSquashedImage(t *testing.T, name string) *image.Image {
	t.Helper()
	imageTarPath := getFixtureImageTarPath(t, name)
	request := fmt.Sprintf("tarball://%s", imageTarPath)

	i, err := stereoscope.GetImage(request)
	if err != nil {
		t.Fatal("could not get tar image", err)
	}

	err = i.Read()
	if err != nil {
		t.Fatal("could not read tar image", err)
	}

	err = i.Squash()
	if err != nil {
		t.Fatal("could not squash image", err)
	}

	return i
}

func getFixtureImageInfo(t *testing.T, name string) (string, string) {
	t.Helper()
	version := fixtureVersion(t, name)
	imageName := fmt.Sprintf("%s-%s", imagePrefix, name)
	return imageName, version

}

func getFixtureImageTarPath(t *testing.T, name string) string {
	t.Helper()
	imageName, imageVersion := getFixtureImageInfo(t, name)
	fullImageName := fmt.Sprintf("%s:%s", imageName, imageVersion)
	tarFileName := fmt.Sprintf("%s.tar", imageName)
	tarPath := path.Join(testFixturesDirName, tarCacheDirName, tarFileName)

	if !fileExists(t, tarPath) {
		if !hasImage(t, fullImageName) {
			contextPath := path.Join(testFixturesDirName, name)
			err := buildImage(t, contextPath, imageName, imageVersion)
			if err != nil {
				t.Fatal("could not build fixture image:", err)
			}
		}

		err := saveImage(t, fullImageName, tarPath)
		if err != nil {
			t.Fatal("could not save fixture image:", err)
		}
	}

	return tarPath
}

func fixtureVersion(t *testing.T, name string) string {
	t.Helper()
	contextPath := path.Join(testFixturesDirName, name)
	dockerfileHash, err := dirHash(t, contextPath)
	if err != nil {
		panic(err)
	}
	return dockerfileHash
}

func fileExists(t *testing.T, filename string) bool {
	t.Helper()
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func dirHash(t *testing.T, root string) (string, error) {
	t.Helper()
	hasher := sha256.New()
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() {
			err := file.Close()
			if err != nil {
				panic(err)
			}
		}()

		if _, err := io.Copy(hasher, file); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}

func hasImage(t *testing.T, imageName string) bool {
	t.Helper()
	cmd := exec.Command("docker", "image", "inspect", imageName)
	cmd.Env = os.Environ()
	err := cmd.Run()
	if err != nil {
		return false
	}
	return true
}

func buildImage(t *testing.T, contextDir, name, tag string) error {
	t.Helper()
	cmd := exec.Command("docker", "build", "-t", name+":"+tag, "-t", name+":latest", ".")
	cmd.Env = os.Environ()
	cmd.Dir = contextDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func saveImage(t *testing.T, image, path string) error {
	t.Helper()

	outfile, err := os.Create(path)
	if err != nil {
		t.Fatal("unable to create file for docker image tar:", err)
	}
	defer func() {
		err := outfile.Close()
		if err != nil {
			panic(err)
		}
	}()

	// note: we are not using -o since some CI providers need root access for the docker client, however,
	// we don't want the resulting tar to be owned by root, thus we write the file piped from stdout.
	cmd := exec.Command("docker", "image", "save", image)
	cmd.Env = os.Environ()

	cmd.Stdout = outfile
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
