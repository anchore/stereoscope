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

type Path string

func (p Path) ID() node.ID {
	return node.ID(p.Normalize())
}

func (p Path) Normalize() Path {
	trimmed := strings.Trim(string(p), " ")
	if trimmed == "/" {
		return Path(trimmed)
	}
	return Path(strings.TrimRight(trimmed, DirSeparator))
}

func (p Path) Basename() string {
	return path.Base(string(p))
}

func (p Path) IsDirWhiteout() bool {
	return p.Basename() == OpaqueWhiteout
}

func (p Path) IsWhiteout() bool {
	return strings.HasPrefix(p.Basename(), WhiteoutPrefix)
}

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
