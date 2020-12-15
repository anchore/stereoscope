package main

import (
	"fmt"
	"os"

	"github.com/anchore/stereoscope"
	"github.com/anchore/stereoscope/pkg/file"
)

func main() {
	// note: we are writing out temp files which should be cleaned up after you're done with the image object
	defer stereoscope.Cleanup()

	/////////////////////////////////////////////////////////////////
	// pass a path to an image tar as an argument:
	//    ./path/to.tar
	//
	// This will catalog the file metadata and resolve all squash trees
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
