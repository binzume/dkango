package dokan

import (
	"errors"
	"io"
	"io/fs"
)

var ErrFailedToLoadDokan = errors.New("Failed to load dokan2.dll")
var ErrDokan = errors.New("Failed to mount")
var ErrBadDriveLetter = errors.New("Bad drive letter")
var ErrDriverInstall = errors.New("Dokan install error")
var ErrStart = errors.New("Dokan start error")
var ErrMount = errors.New("Dokan mount failed")
var ErrBadMountPoint = errors.New("Mount point is invalid")
var ErrDokanVersion = errors.New("Version error")

func ErrorToNTStatus(err error) uintptr {
	if err == nil {
		return STATUS_SUCCESS
	} else if errors.Is(err, fs.ErrNotExist) {
		return STATUS_OBJECT_NAME_NOT_FOUND
	} else if errors.Is(err, fs.ErrExist) {
		return STATUS_OBJECT_NAME_COLLISION
	} else if errors.Is(err, fs.ErrPermission) {
		return STATUS_ACCESS_DENIED
	} else if errors.Is(err, fs.ErrInvalid) {
		return STATUS_INVALID_PARAMETER
	} else if err == io.EOF || errors.Is(err, io.ErrUnexpectedEOF) {
		return STATUS_END_OF_FILE
	}
	return STATUS_ACCESS_DENIED
}
