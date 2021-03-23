package file

import "archive/tar"

const (
	TypeReg             Type = tar.TypeReg
	TypeDir             Type = tar.TypeDir
	TypeSymlink         Type = tar.TypeSymlink
	TypeHardLink        Type = tar.TypeLink
	TypeCharacterDevice Type = tar.TypeChar
	TypeBlockDevice     Type = tar.TypeBlock
	TypeFifo            Type = tar.TypeFifo
)

var AllTypes = []Type{
	TypeReg,
	TypeDir,
	TypeSymlink,
	TypeHardLink,
	TypeCharacterDevice,
	TypeBlockDevice,
	TypeFifo,
}

type Type rune
