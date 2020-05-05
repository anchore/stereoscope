package main

import (
	"fmt"
	"github.com/anchore/stereoscope/stereoscope"
	"github.com/anchore/stereoscope/stereoscope/file"
	"github.com/anchore/stereoscope/stereoscope/tree"
	"io/ioutil"
	"os"
)


func main() {

	client := stereoscope.NewClient()

	// pass a path to an image tar as an argument:
	//    tarball://./path/to.tar
	image, err := client.GetImage(os.Args[1])
	if err != nil {
		panic(err)
	}

	err = image.Read()
	if err != nil {
		panic(err)
	}

	// Example for fetching file contents from the (squashed) image
	reader, err := image.GetFileReader("/etc/centos-release")
	if err != nil {
		panic(err)
	}
	bytes, err := ioutil.ReadAll(reader)
	fmt.Printf("'%+v'\n", string(bytes))

	// Show the filetree for each layer
	for _, l := range image.Layers {
		id, err := l.Content.DiffID()
		if err != nil {
			panic(err)
		}

		fmt.Println("layer", id, ":")

		visitor := image.Structure.VisitorFn(func(f *file.File){
			fmt.Println("   ", f.Path)
		})
		w := tree.NewDepthFirstWalker(l.Structure.Reader(), visitor)
		w.WalkAll()
		fmt.Println("-----------------------------")
	}

	// Show the filetree for the squashed image
	visitor := image.Structure.VisitorFn(func(f *file.File){
		fmt.Println("   ", f.Path)
	})
	w := tree.NewDepthFirstWalker(image.Structure.Reader(), visitor)
	w.WalkAll()

}
