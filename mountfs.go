package dkango

import (
	"io"
	"io/fs"

	"github.com/binzume/dkango/dokan"
)

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
	Flags          uint32
}

const (
	// Enable debug messages
	FlagDebug = dokan.DOKAN_OPTION_DEBUG
	// Output debug messages to stderr
	FlagStderr = dokan.DOKAN_OPTION_STDERR
	// Enable filename:streamname path
	FlagAltStream = dokan.DOKAN_OPTION_ALT_STREAM
	// Readonly FS even if fsys implements OpenWriterFS.
	FlagsWriteProtect = dokan.DOKAN_OPTION_WRITE_PROTECT
	// Network drive
	FlagNetwork = dokan.DOKAN_OPTION_NETWORK
	// Removable drive
	FlagRemovable = dokan.DOKAN_OPTION_REMOVABLE
)

// MountFS mounts fsys on mountPoint.
func MountFS(mountPoint string, fsys fs.FS, opt *MountOptions) (*dokan.MountInfo, error) {
	if opt == nil {
		opt = &MountOptions{
			VolumeInfo:     dokan.VolumeInformation{Name: "", FileSystemName: "Dokan"},
			TotalBytes:     1024 * 1024 * 1024,
			AvailableBytes: 1024 * 1024 * 1024,
			Flags:          dokan.DOKAN_OPTION_ALT_STREAM,
		}
	}
	return dokan.MountDisk(mountPoint, &disk{opt: opt, fsys: fsys}, opt.Flags)
}
