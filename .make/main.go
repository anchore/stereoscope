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
		golint.Tasks(),
		release.Tasks(),
		Task{
			Name:         "integration",
			Description:  "run integration tests",
			Dependencies: Deps("integration-tools"),
			Run: func() {
				Run("go test -v ./test/integration")
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
