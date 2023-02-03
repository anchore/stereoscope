package file

import (
	"archive/tar"
	"os"
)

const (
	TypeReg Type = iota
	TypeHardLink
	TypeSymlink
	TypeCharacterDevice
	TypeBlockDevice
	TypeDir
	TypeFifo
	TypeSocket
	TypeIrregular
)

// why use a rune type? we're looking for something that is memory compact but is easily human interpretable.

type Type int

func AllTypes() []Type {
	return []Type{
		TypeReg,
		TypeHardLink,
		TypeSymlink,
		TypeCharacterDevice,
		TypeBlockDevice,
		TypeDir,
		TypeFifo,
		TypeSocket,
		TypeIrregular,
	}
}

func TypeFromTarType(ty byte) Type {
	switch ty {
	case tar.TypeReg, tar.TypeRegA:
		return TypeReg
	case tar.TypeLink:
		return TypeHardLink
	case tar.TypeSymlink:
		return TypeSymlink
	case tar.TypeChar:
		return TypeCharacterDevice
	case tar.TypeBlock:
		return TypeBlockDevice
	case tar.TypeDir:
		return TypeDir
	case tar.TypeFifo:
		return TypeFifo
	default:
		return TypeIrregular
	}
}

func TypeFromMode(mode os.FileMode) Type {
	switch {
	case isSet(mode, os.ModeSymlink):
		return TypeSymlink
	case isSet(mode, os.ModeIrregular):
		return TypeIrregular
	case isSet(mode, os.ModeCharDevice):
		return TypeCharacterDevice
	case isSet(mode, os.ModeDevice):
		return TypeBlockDevice
	case isSet(mode, os.ModeNamedPipe):
		return TypeFifo
	case isSet(mode, os.ModeSocket):
		return TypeSocket
	case mode.IsDir():
		return TypeDir
	case mode.IsRegular():
		return TypeReg
	default:
		return TypeIrregular
	}
}

func isSet(mode, field os.FileMode) bool {
	return mode&field != 0
}

func (t Type) String() string {
	switch t {
	case TypeReg:
		return "RegularFile"
	case TypeHardLink:
		return "HardLink"
	case TypeSymlink:
		return "SymbolicLink"
	case TypeCharacterDevice:
		return "CharacterDevice"
	case TypeBlockDevice:
		return "BlockDevice"
	case TypeDir:
		return "Directory"
	case TypeFifo:
		return "FIFONode"
	case TypeSocket:
		return "Socket"
	case TypeIrregular:
		return "IrregularFile"
	default:
		return "Unknown"
	}
}
