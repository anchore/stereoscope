package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/anchore/stereoscope"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/filetree/filenode"
	"github.com/sirupsen/logrus"
)

func main() {

	// context for network requests
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	lctx := logrus.New()
	lctx.Level = logrus.DebugLevel
	stereoscope.SetLogger(lctx)

	/////////////////////////////////////////////////////////////////
	// pass a path to an Docker save tar, docker image, or OCI directory/archive as an argument:
	//    ./path/to.tar
	//
	// This will catalog the file metadata and resolve all squash trees
	image, err := stereoscope.GetImage(ctx, os.Args[1])
	if err != nil {
		panic(err)
	}

	// note: we are writing out temp files which should be cleaned up after you're done with the image object
	defer image.Cleanup()

	for _, layer := range image.Layers {
		fmt.Printf("layer: %s\n", layer.Metadata.Digest)
	}

	////////////////////////////////////////////////////////////////
	// Show the filetree for each layer
	for idx, layer := range image.Layers {
		fmt.Printf("Walking layer: %d", idx)
		err = layer.Tree.Walk(func(path file.Path, f filenode.FileNode) error {
			fmt.Println("   ", path)
			return nil
		}, nil)
		fmt.Println("-----------------------------")
		if err != nil {
			panic(err)
		}
	}

	//////////////////////////////////////////////////////////////////
	// Show the squashed filetree for each layer
	for idx, layer := range image.Layers {
		fmt.Printf("Walking squashed layer: %d", idx)
		err = layer.SquashedTree.Walk(func(path file.Path, f filenode.FileNode) error {
			fmt.Println("   ", path)
			return nil
		}, nil)
		fmt.Println("-----------------------------")
		if err != nil {
			panic(err)
		}
	}

	//////////////////////////////////////////////////////////////////
	// Show the final squashed tree
	fmt.Printf("Walking squashed image (same as the last layer squashed tree)")
	err = image.SquashedTree().Walk(func(path file.Path, f filenode.FileNode) error {
		fmt.Println("   ", path)
		return nil
	}, nil)
	if err != nil {
		panic(err)
	}

	//////////////////////////////////////////////////////////////////
	// Fetch file contents from the (squashed) image
	filePath := file.Path("/etc/group")
	contentReader, err := image.FileContentsFromSquash(filePath)
	if err != nil {
		panic(err)
	}

	content, err := ioutil.ReadAll(contentReader)
	if err != nil {
		panic(err)
	}

	fmt.Printf("File content for: %+v\n", filePath)
	fmt.Println(string(content))
}
