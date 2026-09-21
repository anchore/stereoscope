package main

import (
	"os/exec"

	. "github.com/anchore/go-make"
	"github.com/anchore/go-make/run"
	"github.com/anchore/go-make/tasks/golint"
	"github.com/anchore/go-make/tasks/gotest"
	"github.com/anchore/go-make/tasks/release"
)

func main() {
	Makefile(
		gotest.Tasks(gotest.ExcludeGlob("**/test/**")),
		// the containers-storage provider (pkg/image/containerstorage) is only compiled with the
		// containers_image_openpgp build tag; run it as its own suite so the real implementation
		// (not just the stub) is actually compiled and tested in CI.
		gotest.Tasks(
			gotest.Name("unit-containers-storage"),
			// exclude_graphdriver_btrfs avoids needing btrfs/version.h, which CI runners don't have.
			gotest.Tags("containers_image_openpgp", "exclude_graphdriver_btrfs"),
			gotest.IncludeGlob("./pkg/image/containerstorage/..."),
			gotest.NoCoverage(),
		),
		golint.Tasks(),
		release.Tasks(),
		Task{
			Name:        "lint:closecheck",
			Description: "run the closecheck SSA closer-leak analyzer",
			RunsOn:      []string{"lint", "lint-fix", "static-analysis"},
			Run: func() {
				Run("go run ./test/linter/closecheck ./...")
			},
		},
		Task{
			Name:         "integration",
			Description:  "run integration tests",
			Dependencies: Deps("integration-tools"),
			Run: func() {
				// the containers_image_openpgp tag compiles in the containers-storage integration
				// test; it self-skips when buildah isn't on PATH, so this is safe without buildah
				// installed in CI. exclude_graphdriver_btrfs avoids needing btrfs/version.h, which
				// CI runners don't have.
				Run("go test -v -tags containers_image_openpgp,exclude_graphdriver_btrfs ./test/integration")

				// a rootless containers-storage store can only be opened from inside a user namespace, which
				// buildah/podman/skopeo enter by re-execing themselves and a go test binary cannot. the test
				// above self-skips outside of one, so run it again under `buildah unshare` to actually cover it.
				if _, err := exec.LookPath("buildah"); err == nil {
					Run("buildah unshare go test -v -tags containers_image_openpgp,exclude_graphdriver_btrfs -run TestContainersStorageSource ./test/integration")
				}
			},
		},
		Task{
			Name:        "integration-tools",
			Description: "build tools needed for integration tests",
			Run: func() {
				Run("make", run.InDir("test/integration/tools"))
			},
		},
		Task{
			Name:        "integration-tools-load",
			Description: "load tool images needed for integration tests from cache",
			Run: func() {
				Run("make load-cache", run.InDir("test/integration/tools"))
			},
		},
		Task{
			Name:        "integration-tools-save",
			Description: "save tool images needed for integration tests to cache",
			Run: func() {
				Run("make save-cache", run.InDir("test/integration/tools"))
			},
		},
	)
}
