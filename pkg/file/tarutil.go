package file

import (
	"archive/tar"
	"fmt"
	"github.com/anchore/stereoscope/internal"
	"io"
	"io/ioutil"
	"path"
)

type tarFile struct {
	io.Reader
	io.Closer
}

type TarContentsRequest map[string]Reference

func ReaderFromTar(reader io.ReadCloser, tarPath string) (io.ReadCloser, error) {
	tarReader := tar.NewReader(reader)
	for {
		hdr, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Name == tarPath {
			return tarFile{
				Reader: tarReader,
				Closer: reader,
			}, nil
		}
	}
	return nil, fmt.Errorf("file %s not found in tar", tarPath)
}

func ContentsFromTar(reader io.ReadCloser, tarHeaderNames TarContentsRequest) (map[Reference]string, error) {
	result := make(map[Reference]string)
	tarReader := tar.NewReader(reader)

	for {
		hdr, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if fileRef, ok := tarHeaderNames[hdr.Name]; ok{
			bytes, err := ioutil.ReadAll(tarReader)
			if err != nil {
				return nil, fmt.Errorf("could not read file: %s: %+v", hdr.Name, err)
			}
			result[fileRef] = string(bytes)
		}
	}

	if len(result) != len(tarHeaderNames) {
		resultSet := internal.NewStringSet()
		missingNames := make([]string, 0)
		for _, name := range result {
			resultSet.Add(name)
		}
		for name, _ := range tarHeaderNames {
			if !resultSet.Contains(name) {
				missingNames = append(missingNames, name)
			}
		}
		return nil, fmt.Errorf("not all files found: %+v", missingNames)
	}

	return result, nil
}

func EnumerateFileMetadataFromTar(reader io.ReadCloser) <-chan Metadata {
	tarReader := tar.NewReader(reader)
	result := make(chan Metadata)
	go func() {
		for {
			header, err := tarReader.Next()
			if err == io.EOF {
				break
			} else if err != nil {
				panic(err)
			}

			// always ensure relative path notations are not parsed as part of the filename
			name := path.Clean(DirSeparator + header.Name)
			if name == "." {
				continue
			}

			switch header.Typeflag {
			case tar.TypeXGlobalHeader:
				panic(fmt.Errorf("unexptected tar file: (XGlobalHeader): type=%v name=%s", header.Typeflag, name))
			case tar.TypeXHeader:
				panic(fmt.Errorf("unexptected tar file (XHeader): type=%v name=%s", header.Typeflag, name))
			default:
				result <- Metadata{
					Path:          name,
					TarHeaderName: header.Name,
					TypeFlag:      header.Typeflag,
					Linkname:      header.Linkname,
					Size:          header.FileInfo().Size(),
					Mode:          header.FileInfo().Mode(),
					Uid:           header.Uid,
					Gid:           header.Gid,
					IsDir:         header.FileInfo().IsDir(),
				}
			}
		}
		close(result)
	}()
	return result
}
