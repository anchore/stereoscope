package main

import (
	. "github.com/anchore/go-make"
	"github.com/anchore/go-make/tasks/golint"
	"github.com/anchore/go-make/tasks/gotest"
	"github.com/anchore/go-make/tasks/release"
)

func main() {
	Makefile(
		gotest.Tasks(),
		golint.Tasks(),
		release.ChangelogTask(),
		release.TagAndCreateGHRelease(),
	)
}
