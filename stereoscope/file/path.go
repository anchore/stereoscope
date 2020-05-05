package file

import (
	"fmt"
	"github.com/anchore/stereoscope/stereoscope/tree/node"
	"hash/fnv"
	"path"
	"strings"
)

const (
	whiteoutPrefix       = ".wh."
	opaqueWhiteoutPrefix = whiteoutPrefix + whiteoutPrefix + ".opq"
	DirSeparator         = "/"
)

type Path string

func (p Path) ID() node.ID {
	sanitized := p.Normalize()
	h := fnv.New64a()
	_, err := h.Write([]byte(sanitized))
	if err != nil {
		panic(err)
	}
	return node.ID(h.Sum64())
}

func (p Path) Normalize() Path {
	return Path(strings.TrimRight(strings.Trim(string(p), " "), DirSeparator))
}

func (p Path) Basename() string {
	return path.Base(string(p))
}

func (p Path) IsWhiteout() bool {
	return strings.HasPrefix(p.Basename(), whiteoutPrefix)
}

func (p Path) UnWhiteoutPath() (Path, error) {
	basename := p.Basename()
	if strings.HasPrefix(basename, opaqueWhiteoutPrefix) {
		return p.ParentPath()
	}
	parent, err := p.ParentPath()
	if err != nil {
		return "", err
	}
	return Path(path.Join(string(parent), strings.TrimPrefix(basename, whiteoutPrefix))), nil
}

func (p Path) ParentPath() (Path, error) {
	parent, child := path.Split(string(p))
	sanitized := Path(parent).Normalize()
	if sanitized == "" {
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
