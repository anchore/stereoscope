package file

import "archive/tar"

const (
	TypeReg      Type = tar.TypeReg
	TypeDir      Type = tar.TypeDir
	TypeSymlink  Type = tar.TypeSymlink
	TypeHardLink Type = tar.TypeLink
)

type Type rune
