package dkango

import (
	"io"
	"io/fs"

	"github.com/binzume/dkango/dokan"
)

// An interface for opening files for writing.
type OpenWriterFS interface {
	fs.FS
	OpenWriter(name string, flag int) (io.WriteCloser, error)
}

// An interface to remove file or directory from the file system.
type RemoveFS interface {
	fs.FS
	Remove(name string) error
}

// An interface to rename file or directory from the file system.
type RenameFS interface {
	fs.FS
	Rename(name string, newName string) error
}

// An interface to make new directories in the file system.
type MkdirFS interface {
	fs.FS
	Mkdir(name string, mode fs.FileMode) error
}

// An interface for preferentially opening ReadDirFile.
// If OpenDirFS is not implemented, try using fs.ReadDirFS, then Open file and try using fs.ReadDirFile.
type OpenDirFS interface {
	fs.FS
	OpenDir(name string) (fs.ReadDirFile, error)
}

// An interface to truncate file to specified size.
// If TruncateFS is not implemented, open file and try using file.Truncate(size).
type TruncateFS interface {
	fs.FS
	Truncate(name string, size int64) error
}

// DiskSpace represents the amount of space that is available on a disk.
// https://docs.microsoft.com/ja-JP/windows/win32/api/fileapi/nf-fileapi-getdiskfreespaceexa
type DiskSpace struct {
	FreeBytesAvailable     uint64
	TotalNumberOfBytes     uint64
	TotalNumberOfFreeBytes uint64
}

type MountOptions struct {
	VolumeInfo    dokan.VolumeInformation
	DiskSpaceFunc func() DiskSpace // optional
	Flags         uint32
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
//
// mountPoint must be a valid unused drive letter or a directory on NTFS.
//
// To provide random access, file opened by fsys should implement io.Seeker or ReaderAt and WriterAt.
// If only sequential access is provided, many applications will not work properly.
func MountFS(mountPoint string, fsys fs.FS, opt *MountOptions) (*dokan.MountInfo, error) {
	if opt == nil {
		opt = &MountOptions{
			VolumeInfo: dokan.VolumeInformation{Name: "", FileSystemName: "Dokan"},
			Flags:      dokan.DOKAN_OPTION_ALT_STREAM,
		}
	}
	return dokan.MountDisk(mountPoint, &disk{opt: opt, fsys: fsys}, opt.Flags)
}
