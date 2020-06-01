package main

import (
	"fmt"
	"os"

	"github.com/anchore/stereoscope"
	"github.com/anchore/stereoscope/pkg/file"
)

func main() {
	/////////////////////////////////////////////////////////////////
	// pass a path to an image tar as an argument:
	//    tarball://./path/to.tar
	//
	// This will:
	//  - catalog the file metadata (img.Read)
	//  - squash the layers into a single filetree (img.Squash)
	//
	// note: if you'd prefer to read and squash the image manually, pass stereoscope.NoActionOption
	// or pass the Option you'd prefer (e.g. only read, no squash = stereoscope.ReadImageOption)
	image, err := stereoscope.GetImage(os.Args[1])
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
	// Show the squashed tree
	image.SquashedTree.Walk(func(f file.Reference) {
		fmt.Println("   ", f.Path)
	})

	////////////////////////////////////////////////////////////////
	// Fetch file contents from the (squashed) image
	content, err := image.FileContentsFromSquash("/etc/centos-release")
	if err != nil {
		panic(err)
	}
	fmt.Println(content)
}
