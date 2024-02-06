package imagetest

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"testing"

	"github.com/logrusorgru/aurora"
	"github.com/stretchr/testify/require"

	"github.com/anchore/go-testutils"
	"github.com/anchore/stereoscope"
	"github.com/anchore/stereoscope/internal/containerd"
	"github.com/anchore/stereoscope/pkg/image"
)

const (
	CacheDir    = testutils.TestFixturesDir + string(filepath.Separator) + "cache"
	ImagePrefix = "stereoscope-fixture"
)

func PrepareFixtureImage(t testing.TB, source, name string) string {
	t.Helper()

	var location string
	switch source {
	case image.ContainerdDaemonSource:
		location = LoadFixtureImageIntoContainerd(t, name)
	case image.DockerTarballSource:
		location = GetFixtureImageTarPath(t, name)
	case image.DockerDaemonSource:
		location = LoadFixtureImageIntoDocker(t, name)
	case image.PodmanDaemonSource:
		location = LoadFixtureImageIntoPodman(t, name)
	case image.OciTarballSource:
		dockerArchivePath := GetFixtureImageTarPath(t, name)
		ociArchivePath := path.Join(path.Dir(dockerArchivePath), "oci-archive-"+path.Base(dockerArchivePath))
		if _, err := os.Stat(ociArchivePath); os.IsNotExist(err) {
			skopeoCopyDockerArchiveToPath(t, dockerArchivePath, fmt.Sprintf("oci-archive:%s", ociArchivePath))
		}
		location = ociArchivePath
	case image.OciDirectorySource:
		dockerArchivePath := GetFixtureImageTarPath(t, name)
		ociDirPath := path.Join(path.Dir(dockerArchivePath), "oci-dir-"+path.Base(dockerArchivePath))
		if _, err := os.Stat(ociDirPath); os.IsNotExist(err) {
			skopeoCopyDockerArchiveToPath(t, dockerArchivePath, fmt.Sprintf("oci:%s", ociDirPath))
		}
		location = ociDirPath
	case image.SingularitySource:
		location = GetFixtureImageSIFPath(t, name)
	default:
		t.Fatalf("could not determine source: %+v", source)
	}

	return fmt.Sprintf("%s:%s", source, location)
}

func GetFixtureImage(t testing.TB, source, name string) *image.Image {
	request := PrepareFixtureImage(t, source, name)

	i, err := stereoscope.GetImage(context.TODO(), request)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, i.Cleanup())
	})
	return i
}

func GetGoldenFixtureImage(t testing.TB, name string) *image.Image {
	imageName, _ := getFixtureImageInfo(t, name)
	tarFileName := imageName + testutils.GoldenFileExt
	tarPath := getFixtureImageTarPath(t, name, testutils.GoldenFileDirPath, tarFileName)
	return getFixtureImageFromTar(t, tarPath)
}

func UpdateGoldenFixtureImage(t testing.TB, name string) {
	t.Log(aurora.Reverse(aurora.Red("!!! UPDATING GOLDEN FIXTURE IMAGE !!!")), name)

	imageName, _ := getFixtureImageInfo(t, name)
	goldenTarFilePath := path.Join(testutils.GoldenFileDirPath, imageName+testutils.GoldenFileExt)
	tarPath := GetFixtureImageTarPath(t, name)
	copyFile(t, tarPath, goldenTarFilePath)
}

func isSkopeoAvailable() bool {
	_, err := exec.LookPath("skopeo")
	return err == nil
}

func skopeoCopyDockerArchiveToPath(t testing.TB, dockerArchivePath, destination string) {
	if !isSkopeoAvailable() {
		t.Fatalf("cannot find skopeo executable")
	}

	archive := fmt.Sprintf("docker-archive:%s", dockerArchivePath)
	cmd := exec.Command("skopeo", "copy", "--insecure-policy", archive, destination)
	cmd.Env = os.Environ()

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		t.Fatalf("skopeo failed: %+v", err)
	}
}

func getFixtureImageFromTar(t testing.TB, tarPath string) *image.Image {
	request := fmt.Sprintf("docker-archive:%s", tarPath)

	i, err := stereoscope.GetImage(context.TODO(), request)
	require.NoError(t, err)

	t.Cleanup(func() {
		if err := i.Cleanup(); err != nil {
			t.Errorf("could not cleanup tarPath=%q: %+v", tarPath, err)
		}
	})

	return i
}

func getFixtureImageInfo(t testing.TB, name string) (string, string) {
	version := fixtureVersion(t, name)
	imageName := fmt.Sprintf("%s-%s", ImagePrefix, name)
	return imageName, version
}

func LoadFixtureImageIntoDocker(t testing.TB, name string) string {
	return loadFixtureInContainerEngine(t, name, isImageInDocker, buildDockerImage)
}

func LoadFixtureImageIntoPodman(t testing.TB, name string) string {
	return loadFixtureInContainerEngine(t, name, isImageInPodman, buildPodmanImage)
}

func LoadFixtureImageIntoContainerd(t testing.TB, name string) string {
	return loadFixtureInContainerEngine(t, name, isImageInContainerd, buildContainerdImage)
}

func loadFixtureInContainerEngine(t testing.TB, name string,
	hasImage func(string) bool, build func(testing.TB, string, string, string)) string {
	imageName, imageVersion := getFixtureImageInfo(t, name)
	fullImageName := fmt.Sprintf("%s:%s", imageName, imageVersion)

	if !hasImage(fullImageName) {
		contextPath := path.Join(testutils.TestFixturesDir, name)
		build(t, contextPath, imageName, imageVersion)
	}

	return fullImageName
}

func getFixtureImageTarPath(t testing.TB, fixtureName, tarStoreDir, tarFileName string) string {
	imageName, imageVersion := getFixtureImageInfo(t, fixtureName)
	fullImageName := fmt.Sprintf("%s:%s", imageName, imageVersion)
	tarPath := path.Join(tarStoreDir, tarFileName)

	// create the cache dir if it does not already exist...
	if !fileOrDirExists(t, CacheDir) {
		err := os.Mkdir(CacheDir, 0o755)
		if err != nil {
			t.Fatalf("could not create tar cache dir (%s): %+v", CacheDir, err)
		}
	}

	// if the image tar does not exist, make it
	if !fileOrDirExists(t, tarPath) {
		if !isImageInDocker(fullImageName) {
			contextPath := path.Join(testutils.TestFixturesDir, fixtureName)
			buildDockerImage(t, contextPath, imageName, imageVersion)
		}

		err := saveImage(t, fullImageName, tarPath)
		if err != nil {
			t.Fatal("could not save fixture image:", err)
		}
	}

	return tarPath
}

func GetFixtureImageTarPath(t testing.TB, name string) string {
	imageName, imageVersion := getFixtureImageInfo(t, name)
	tarFileName := fmt.Sprintf("%s-%s.tar", imageName, imageVersion)
	return getFixtureImageTarPath(t, name, CacheDir, tarFileName)
}

func fixtureVersion(t testing.TB, name string) string {
	contextPath := path.Join(testutils.TestFixturesDir, name)
	dockerfileHash := dirHash(t, contextPath)
	return dockerfileHash
}

func isImageInDocker(imageName string) bool {
	cmd := exec.Command("docker", "image", "inspect", imageName)
	cmd.Env = os.Environ()
	err := cmd.Run()
	return err == nil
}

func isImageInPodman(imageName string) bool {
	cmd := exec.Command("podman", "image", "inspect", imageName)
	cmd.Env = os.Environ()
	err := cmd.Run()
	return err == nil
}

func isImageInContainerd(imageName string) bool {
	cmd := exec.Command("ctr", "image", "inspect", "|", "grep", imageName)
	cmd.Env = os.Environ()
	err := cmd.Run()
	return err == nil
}

func buildDockerImage(t testing.TB, contextDir, name, tag string) {
	t.Logf("Build docker image: name=%q tag=%q", name, tag)
	fullTag := fmt.Sprintf("%s:%s", name, tag)
	latestTag := fmt.Sprintf("%s:latest", name)
	cmd := exec.Command("docker", "build", "-t", fullTag, "-t", latestTag, ".")
	cmd.Env = os.Environ()
	cmd.Dir = contextDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	require.NoError(t, cmd.Run(), "could not build docker image (shell out)")
}

func buildPodmanImage(t testing.TB, contextDir, name, tag string) {
	t.Logf("Build podman image: name=%q tag=%q", name, tag)

	fullTag := fmt.Sprintf("%s:%s", name, tag)
	latestTag := fmt.Sprintf("%s:latest", name)
	cmd := exec.Command("podman", "build", "-t", fullTag, "-t", latestTag, ".")
	cmd.Env = os.Environ()
	cmd.Dir = contextDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	require.NoError(t, cmd.Run(), "could not build podman image (shell out)")
}

func buildContainerdImage(t testing.TB, contextDir, name, tag string) {
	fullTag := fmt.Sprintf("%s:%s", name, tag)
	tempFile := fmt.Sprintf("/tmp/%s.tar.gz", fullTag)
	buildDockerImage(t, contextDir, name, tag)

	err := saveImage(t, fullTag, tempFile)
	require.NoError(t, err, "could not save docker image (shell out)")
	cmd := exec.Command("ctr", "image", "import", tempFile)

	env := os.Environ()
	if os.Getenv("CONTAINERD_ADDRESS") == "" {
		env = append(env, fmt.Sprintf("CONTAINERD_ADDRESS=%s", containerd.Address()))
	}
	cmd.Env = env

	cmd.Dir = contextDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	require.NoError(t, cmd.Run(), "could not import docker image to containerd (shell out)")
	require.NoError(t, os.Remove(tempFile), "could not remove saved docker image")
}

func saveImage(t testing.TB, image, path string) error {
	outfile, err := os.Create(path)
	if err != nil {
		t.Fatal("unable to create file for docker image tar:", err)
	}
	defer func() {
		err := outfile.Close()
		if err != nil {
			t.Fatalf("unable to close file path=%q : %+v", path, err)
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

func GetFixtureImageSIFPath(t testing.TB, name string) string {
	imageName, imageVersion := getFixtureImageInfo(t, name)
	sifFileName := fmt.Sprintf("%s-%s.sif", imageName, imageVersion)
	return getFixtureImageSIFPath(t, name, CacheDir, sifFileName)
}

func getFixtureImageSIFPath(t testing.TB, fixtureName, sifStoreDir, sifFileName string) string {
	imageName, imageVersion := getFixtureImageInfo(t, fixtureName)
	fullImageName := fmt.Sprintf("%s:%s", imageName, imageVersion)
	sifPath := path.Join(sifStoreDir, sifFileName)

	// create the cache dir if it does not already exist...
	if !fileOrDirExists(t, CacheDir) {
		err := os.Mkdir(CacheDir, 0o755)
		if err != nil {
			t.Fatalf("could not create sif cache dir (%s): %+v", CacheDir, err)
		}
	}

	// if the image sif does not exist, make it
	if !fileOrDirExists(t, sifPath) {
		if !isImageInDocker(fullImageName) {
			contextPath := path.Join(testutils.TestFixturesDir, fixtureName)
			buildDockerImage(t, contextPath, imageName, imageVersion)
		}
		err := buildSIFFromDocker(t, fullImageName, sifPath)
		if err != nil {
			t.Fatal("could not save fixture image:", err)
		}
	}

	return sifPath
}

func buildSIFFromDocker(t testing.TB, image, path string) error {
	absHostDir, err := filepath.Abs(filepath.Dir(path))
	require.NoError(t, err)

	singularityArgs := []string{"build", "--disable-cache", "--force", "image/" + filepath.Base(path), "docker-daemon:" + image}

	allArgs := append([]string{
		"run",
		"-t",
		"--rm",
		"-v",
		"/var/run/docker.sock:/var/run/docker.sock",
		"-v",
		absHostDir + ":/image",
		"localhost/singularity:dev", // from integration tools (make integration-tools)
		"singularity",
	}, singularityArgs...)

	cmd := exec.Command("docker", allArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
