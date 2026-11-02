// Package filenode is a hand-built stand-in for stereoscope's filenode package, with just the fields the analyzer
// looks at, so analysistest can load it from testdata.
package filenode

type Reference struct {
	RealPath string
}

type FileNode struct {
	RealPath  string
	FileType  int
	Reference *Reference
}
