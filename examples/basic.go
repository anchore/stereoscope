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
	// This will catalog the file metadata and resolve all squash trees (img.Read)
	//
	// note: if you'd prefer to read the image manually, pass stereoscope.NoActionOption
	image, err := stereoscope.GetImage(os.Args[1])
	if err != nil {
		panic(err)
	}

	////////////////////////////////////////////////////////////////
	// Show the filetree for each layer
	for idx, layer := range image.Layers {
		fmt.Printf("Walking layer: %d", idx)
		layer.Tree.Walk(func(f file.Reference) {
			fmt.Println("   ", f.Path)
		})
		fmt.Println("-----------------------------")
	}

	////////////////////////////////////////////////////////////////
	// Show the squashed filetree for each layer
	for idx, layer := range image.Layers {
		fmt.Printf("Walking squashed layer: %d", idx)
		layer.SquashedTree.Walk(func(f file.Reference) {
			fmt.Println("   ", f.Path)
		})
		fmt.Println("-----------------------------")
	}

	////////////////////////////////////////////////////////////////
	// Show the final squashed tree
	fmt.Printf("Walking squashed image (same as the last layer squashed tree)")
	image.SquashedTree().Walk(func(f file.Reference) {
		fmt.Println("   ", f.Path)
	})

	////////////////////////////////////////////////////////////////
	// Fetch file contents from the (squashed) image
	content, err := image.FileContentsFromSquash("/etc/group")
	if err != nil {
		panic(err)
	}
	fmt.Println(content)
}
