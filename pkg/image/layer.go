package image

import (
	"archive/tar"
	"fmt"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/tree"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"io"
	"path"
)

type Layer struct {
	Index     uint
	Content   v1.Layer
	Structure *tree.FileTree
}

func ReadLayer(index uint, content v1.Layer, catalog *FileCatalog) (Layer, error) {
	layer := Layer{
		Content: content,
	}
	fileTree := tree.NewFileTree()

	reader, err := content.Uncompressed()
	if err != nil {
		return Layer{}, err
	}
	tarReader := tar.NewReader(reader)

	for metadata := range enumerateFileMetadata(tarReader) {
		fileNode, err := fileTree.AddPath(file.Path(metadata.Path))

		catalog.Add(fileNode, metadata, &layer)

		if err != nil {
			return Layer{}, err
		}
	}
	return Layer{
		Index:     index,
		Content:   content,
		Structure: fileTree,
	}, nil
}

func enumerateFileMetadata(tarReader *tar.Reader) <-chan file.Metadata {
	result := make(chan file.Metadata)
	go func() {
		for {
			header, err := tarReader.Next()
			if err == io.EOF {
				break
			} else if err != nil {
				panic(err)
			}

			// always ensure relative path notations are not parsed as part of the filename
			name := path.Clean(file.DirSeparator + header.Name)
			if name == "." {
				continue
			}

			switch header.Typeflag {
			case tar.TypeXGlobalHeader:
				panic(fmt.Errorf("unexptected tar file: (XGlobalHeader): type=%v name=%s", header.Typeflag, name))
			case tar.TypeXHeader:
				panic(fmt.Errorf("unexptected tar file (XHeader): type=%v name=%s", header.Typeflag, name))
			default:
				result <- file.Metadata{
					Path:     name,
					TarPath:  header.Name,
					TypeFlag: header.Typeflag,
					Linkname: header.Linkname,
					Size:     header.FileInfo().Size(),
					Mode:     header.FileInfo().Mode(),
					Uid:      header.Uid,
					Gid:      header.Gid,
					IsDir:    header.FileInfo().IsDir(),
				}
			}
		}
		close(result)
	}()
	return result
}
