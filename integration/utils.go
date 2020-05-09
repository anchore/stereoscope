// +build integration

package integration

import (
	"crypto/sha256"
	"fmt"
	"github.com/anchore/stereoscope"
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

func compareLayerTrees(t *testing.T, expected map[uint]*tree.FileTree, i *image.Image) {
	t.Helper()
	if len(expected) != len(i.Layers) {
		t.Fatalf("mismatched layers (%d!=%d)", len(expected), len(i.Layers))
	}

	for idx, expected := range expected {
		actual := i.Layers[idx].Tree
		if !expected.Equal(actual) {
			fmt.Println(expected.PathDiff(actual))
			t.Errorf("mismatched trees (layer %d)", idx)
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

func getFixtureImageName(t *testing.T, name string) string {
	t.Helper()
	contextPath := path.Join(testFixturesDirName, name)
	version := fixtureVersion(t, name)
	imageName := fmt.Sprintf("%s-%s", imagePrefix, name)
	fullImageName := fmt.Sprintf("%s:%s", imageName, version)
	if !hasImage(t, fullImageName) {
		err := buildImage(t, contextPath, imageName, version)
		if err != nil {
			panic(err)
		}
	}
	return fullImageName
}

func getFixtureImageTarPath(t *testing.T, name string) string {
	t.Helper()
	imageName := getFixtureImageName(t, name)
	tarFileName := fmt.Sprintf("%s.tar", imageName)
	tarPath := path.Join(testFixturesDirName, tarCacheDirName, tarFileName)

	if !fileExists(t, tarPath) {
		err := saveImage(t, imageName, tarPath)
		if err != nil {
			panic(err)
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
	cmd := exec.Command("docker", "image", "save", image, "-o", path)
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
