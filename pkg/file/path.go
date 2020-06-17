package file

import (
	"fmt"
	"path"
	"strings"

	"github.com/anchore/stereoscope/pkg/tree/node"
)

const (
	WhiteoutPrefix = ".wh."
	OpaqueWhiteout = WhiteoutPrefix + WhiteoutPrefix + ".opq"
	DirSeparator   = "/"
)

// Path represents a file path
type Path string

// ID is the normalized file path, used for file tree node identification
func (p Path) ID() node.ID {
	return node.ID(p.Normalize())
}

// Normalize returns the cleaned file path representation (trimmed of spaces)
func (p Path) Normalize() Path {
	trimmed := strings.Trim(string(p), " ")
	if trimmed == "/" {
		return Path(trimmed)
	}
	return Path(strings.TrimRight(trimmed, DirSeparator))
}

// Basename of the path (i.e. filename)
func (p Path) Basename() string {
	return path.Base(string(p))
}

// IsDirWhiteout indicates if the path has a basename is a opaque whiteout (which means all parent directory contents should be ignored during squashing)
func (p Path) IsDirWhiteout() bool {
	return p.Basename() == OpaqueWhiteout
}

// IsWhiteout indicates if the file basename has a whiteout prefix (which means that the file should be removed during squashing)
func (p Path) IsWhiteout() bool {
	return strings.HasPrefix(p.Basename(), WhiteoutPrefix)
}

// UnWhiteoutPath is a representation of the current path with no whiteout prefixes
func (p Path) UnWhiteoutPath() (Path, error) {
	basename := p.Basename()
	if strings.HasPrefix(basename, OpaqueWhiteout) {
		return p.ParentPath()
	}
	parent, err := p.ParentPath()
	if err != nil {
		return "", err
	}
	return Path(path.Join(string(parent), strings.TrimPrefix(basename, WhiteoutPrefix))), nil
}

// ParentPath returns a Path object to the current files parent directory (or errors out if there is no parent)
func (p Path) ParentPath() (Path, error) {
	parent, child := path.Split(string(p))
	sanitized := Path(parent).Normalize()
	if sanitized == "/" {
		if child != "" {
			return "/", nil
		}
		return "", fmt.Errorf("no parent")
	}
	return sanitized, nil
}

// AllPaths returns all valid constituent paths (e.g. /home/wagoodman/file.txt -> /, /home, /home/wagoodman )
func (p Path) AllPaths() []Path {
	parents := strings.Split(strings.Trim(string(p), DirSeparator), DirSeparator)
	fullPaths := make([]Path, len(parents)+1)
	for idx := range parents {
		cur := DirSeparator + strings.Join(parents[:idx], DirSeparator)
		fullPaths[idx] = Path(cur)
	}
	fullPaths[len(parents)] = p
	return fullPaths
}
