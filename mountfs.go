package dkango

import (
	"io"
	"io/fs"

	"github.com/binzume/dkango/dokan"
)

// You can change this before MountFS() for debugging purpose.
var OptionFlags uint32 = dokan.DOKAN_OPTION_ALT_STREAM // | DOKAN_OPTION_DEBUG | DOKAN_OPTION_STDERR

type OpenWriterFS interface {
	fs.FS
	OpenWriter(name string, flag int) (io.WriteCloser, error)
}

type RemoveFS interface {
	fs.FS
	Remove(name string) error
}

type RenameFS interface {
	fs.FS
	Rename(name string, newName string) error
}

type MkdirFS interface {
	fs.FS
	Mkdir(name string, mode fs.FileMode) error
}

type OpenDirFS interface {
	fs.FS
	OpenDir(name string) (fs.ReadDirFile, error)
}

type TruncateFS interface {
	fs.FS
	Truncate(name string, size int64) error
}

type MountOptions struct {
	VolumeInfo     dokan.VolumeInformation
	TotalBytes     uint64
	AvailableBytes uint64
	WriteProtect   bool // Readonly FS even if fsys implements OpenWriterFS.
}

// MountFS mounts fsys on mountPoint.
func MountFS(mountPoint string, fsys fs.FS, opt *MountOptions) (*dokan.MountInfo, error) {
	if opt == nil {
		opt = &MountOptions{
			VolumeInfo:     dokan.VolumeInformation{Name: "", FileSystemName: "Dokan"},
			TotalBytes:     1024 * 1024 * 1024,
			AvailableBytes: 1024 * 1024 * 1024,
		}
	}
	var flags uint32
	if opt.WriteProtect {
		flags = dokan.DOKAN_OPTION_WRITE_PROTECT
	}
	return dokan.MountDisk(mountPoint, &disk{opt: opt, fsys: fsys}, OptionFlags|flags)
}
