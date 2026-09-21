package main

import (
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
