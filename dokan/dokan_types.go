package dokan

import (
	"strconv"
	"unsafe"
)

// DokanOptions
const (
	DOKAN_OPTION_DEBUG              = 1
	DOKAN_OPTION_STDERR             = 2
	DOKAN_OPTION_ALT_STREAM         = 4
	DOKAN_OPTION_WRITE_PROTECT      = 8
	DOKAN_OPTION_NETWORK            = 16
	DOKAN_OPTION_REMOVABLE          = 32
	DOKAN_OPTION_MOUNT_MANAGER      = 64
	DOKAN_OPTION_CURRENT_SESSION    = 128
	DOKAN_OPTION_FILELOCK_USER_MODE = 256
)

type NTStatus uintptr

func (n NTStatus) Error() string {
	return "NTSTATUS=" + strconv.FormatUint(uint64(n), 16)
}

// NTSTATUS
const (
	STATUS_SUCCESS               = NTStatus(0)
	STATUS_INVALID_PARAMETER     = NTStatus(0xC000000D)
	STATUS_END_OF_FILE           = NTStatus(0xC0000011)
	STATUS_ACCESS_DENIED         = NTStatus(0xC0000022)
	STATUS_OBJECT_NAME_NOT_FOUND = NTStatus(0xC0000034)
	STATUS_OBJECT_NAME_COLLISION = NTStatus(0xC0000035)
	STATUS_OBJECT_PATH_NOT_FOUND = NTStatus(0xC000003A)
	STATUS_FILE_IS_A_DIRECTORY   = NTStatus(0xC00000BA)
	STATUS_NOT_SAME_DEVICE       = NTStatus(0xC00000D4)
	STATUS_NOT_SUPPORTED         = NTStatus(0xC00000BB)
	STATUS_DIRECTORY_NOT_EMPTY   = NTStatus(0xC0000101)
	STATUS_NOT_A_DIRECTORY       = NTStatus(0xC0000103)
)

// File attribute
const (
	FILE_ATTRIBUTE_READONLY  = 1
	FILE_ATTRIBUTE_HIDDEN    = 2
	FILE_ATTRIBUTE_SYSTEM    = 4
	FILE_ATTRIBUTE_DIRECTORY = 16
	FILE_ATTRIBUTE_ARCHIVE   = 32
	FILE_ATTRIBUTE_NORMAL    = 128
)

// ZwCreateFile options
// https://docs.microsoft.com/en-us/windows/win32/api/winternl/nf-winternl-ntcreatefile
const (
	// options
	FILE_DIRECTORY_FILE     = 0x01
	FILE_WRITE_THROUGH      = 0x02
	FILE_SEQUENTIAL_ONLY    = 0x04
	FILE_NON_DIRECTORY_FILE = 0x40
	FILE_DELETE_ON_CLOSE    = 0x000010

	// disposition
	FILE_SUPERSEDE    = 0
	FILE_OPEN         = 1
	FILE_CREATE       = 2
	FILE_OPEN_IF      = 3
	FILE_OVERWRITE    = 4
	FILE_OVERWRITE_IF = 5

	// access
	FILE_READ_DATA        = 1
	FILE_WRITE_DATA       = 2
	FILE_APPEND_DATA      = 4
	FILE_READ_EA          = 8
	FILE_WRITE_EA         = 0x10
	FILE_EXECUTE          = 0x20
	FILE_DELETE_CHILD     = 0x40
	FILE_READ_ATTRIBUTES  = 0x80
	FILE_WRITE_ATTRIBUTES = 0x100
)

const VOLUME_SECURITY_DESCRIPTOR_MAX_SIZE = (1024 * 16)
const MAX_PATH = 260

type MountPointInfo struct {
	Type         uint32
	MountPoint   string
	UNCName      string
	DeviceName   string
	SessionID    uint32
	MountOptions uint32
}

type nativeMountPointInfo struct {
	Type         uint32
	MountPoint   [MAX_PATH]uint16
	UNCName      [64]uint16
	DeviceName   [64]uint16
	SessionID    uint32
	MountOptions uint32
}

type DokanOptions struct {
	Version       uint16
	SingleThread  uint8
	Options       uint32
	GlobalContext unsafe.Pointer

	MountPoint uintptr // LPCWSTR
	UNCName    uintptr // LPCWSTR

	Timeout                        uint32
	AllocationUnitSize             uint32
	SectorSize                     uint32
	VolumeSecurityDescriptorLength uint32
	VolumeSecurityDescriptor       [VOLUME_SECURITY_DESCRIPTOR_MAX_SIZE]byte
}

type DokanOperations struct {
	ZwCreateFile         uintptr
	Cleanup              uintptr
	CloseFile            uintptr
	ReadFile             uintptr
	WriteFile            uintptr
	FlushFileBuffers     uintptr
	GetFileInformation   uintptr
	FindFiles            uintptr
	FindFilesWithPattern uintptr
	SetFileAttributes    uintptr
	SetFileTime          uintptr
	DeleteFile           uintptr
	DeleteDirectory      uintptr
	MoveFile             uintptr
	SetEndOfFile         uintptr
	SetAllocationSize    uintptr

	LockFile   uintptr
	UnlockFile uintptr

	GetDiskFreeSpace     uintptr
	GetVolumeInformation uintptr
	Mounted              uintptr
	Unmounted            uintptr

	GetFileSecurity uintptr
	SetFileSecurity uintptr

	FindStreams uintptr
}

type FileInfo struct {
	Context           unsafe.Pointer
	DokanContext      uint64
	DokanOptions      *DokanOptions
	ProcessingContext uintptr
	ProcessId         uint32
	IsDirectory       uint8
	DeleteOnClose     uint8
	PagingIo          uint8
	SynchronousIo     uint8
	Nocache           uint8
	WriteToEndOfFile  uint8
}

func (fi *FileInfo) IsDeleteOnClose() bool {
	return fi.DeleteOnClose != 0
}

type FileTime [2]uint32 // TODO

type ByHandleFileInfo struct {
	FileAttributes     int32
	CreationTime       FileTime
	LastAccessTime     FileTime
	LastWriteTime      FileTime
	VolumeSerialNumber int32
	FileSizeHigh       uint32
	FileSizeLow        uint32
	NumberOfLinks      int32
	FileIndexHigh      int32
	FileIndexLow       int32
}

type WIN32_FIND_DATAW struct {
	FileAttributes    int32
	CreationTime      FileTime
	LastAccessTime    FileTime
	LastWriteTime     FileTime
	FileSizeHigh      uint32
	FileSizeLow       uint32
	Reserved0         int32
	Reserved1         int32
	FileName          [MAX_PATH]uint16
	AlternateFileName [14]uint16
	dwFileType        int32
	dwCreatorType     int32
	wFinderFlags      int16
}
