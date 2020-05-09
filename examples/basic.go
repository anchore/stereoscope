package main

import (
	"fmt"
	"github.com/anchore/stereoscope"
	"github.com/anchore/stereoscope/pkg/file"
	"os"
)

func main() {
	/////////////////////////////////////////////////////////////////
	// pass a path to an image tar as an argument:
	//    tarball://./path/to.tar
	image, err := stereoscope.GetImage(os.Args[1])
	if err != nil {
		panic(err)
	}

	/////////////////////////////////////////////////////////////////
	// Catalog the file metadata and build the file trees
	err = image.Read()
	if err != nil {
		panic(err)
	}

	////////////////////////////////////////////////////////////////
	// Show the filetree for each layer
	for _, layer := range image.Layers {
		layer.Tree.Walk(func(f file.Reference) {
			fmt.Println("   ", f.Path)
		})
		fmt.Println("-----------------------------")
	}

	////////////////////////////////////////////////////////////////
	// Squash the file trees
	err = image.Squash()
	if err != nil {
		panic(err)
	}

	////////////////////////////////////////////////////////////////
	// Show the squashed tree
	image.SquashedTree.Walk(func(f file.Reference) {
		fmt.Println("   ", f.Path)
	})

	////////////////////////////////////////////////////////////////
	// Fetch file contents from the (squashed) image
	content, err := image.FileContents("/etc/centos-release")
	if err != nil {
		panic(err)
	}
	fmt.Println(content)

}
