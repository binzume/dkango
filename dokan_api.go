package dkango

import (
	"errors"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const DOKAN_MINIMUM_COMPATIBLE_VERSION = 200
const VOLUME_SECURITY_DESCRIPTOR_MAX_SIZE = (1024 * 16)
const MAX_PATH = 260

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

const (
	STATUS_SUCCESS               = 0
	STATUS_INVALID_PARAMETER     = 0xC000000D
	STATUS_END_OF_FILE           = 0xC0000011
	STATUS_ACCESS_DENINED        = 0xC0000022
	STATUS_OBJECT_NAME_NOT_FOUND = 0xC0000034
	STATUS_FILE_IS_A_DIRECTORY   = 0xC00000BA
	STATUS_NOT_SUPPORTED         = 0xC00000BB
	STATUS_NOT_A_DIRECTORY       = 0xC0000103
)

const (
	FILE_ATTRIBUTE_READONLY  = 1
	FILE_ATTRIBUTE_HIDDEN    = 2
	FILE_ATTRIBUTE_SYSTEM    = 4
	FILE_ATTRIBUTE_DIRECTORY = 16
	FILE_ATTRIBUTE_ARCHIVE   = 32
	FILE_ATTRIBUTE_NORMAL    = 128

	FILE_FLAG_DELETE_ON_CLOSE = 0x04000000
)

type MountPointInfo struct {
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
	GlobalContext uint64

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

type DokanFileInfo struct {
	Context           uint64
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

var (
	dokan2             = windows.NewLazySystemDLL("dokan2.dll")
	dokanDriverVersion = dokan2.NewProc("DokanDriverVersion")
	dokanVersion       = dokan2.NewProc("DokanVersion")
	dokanInit          = dokan2.NewProc("DokanInit")
	dokanShutdown      = dokan2.NewProc("DokanShutdown")

	dokanMain                    = dokan2.NewProc("DokanMain")
	dokanCreateFileSystem        = dokan2.NewProc("DokanCreateFileSystem")
	dokanCloseHandle             = dokan2.NewProc("DokanCloseHandle")
	dokanWaitForFileSystemClosed = dokan2.NewProc("DokanWaitForFileSystemClosed")
	dokanRemoveMountPoint        = dokan2.NewProc("DokanRemoveMountPoint")
	dokanOpenRequestorToken      = dokan2.NewProc("DokanOpenRequestorToken")

	dokanGetMountPointList     = dokan2.NewProc("DokanGetMountPointList")
	dokanReleaseMountPointList = dokan2.NewProc("DokanReleaseMountPointList")

	dokanResetTimeout = dokan2.NewProc("DokanResetTimeout")
)

func convErr(err syscall.Errno) error {
	if err == 0 {
		return nil
	}
	return err
}

func DriverVersion() (uint32, error) {
	if dokanDriverVersion.Find() != nil {
		return 0, errors.New("Failed to load dokan2.dll")
	}
	ret, _, err := syscall.SyscallN(dokanDriverVersion.Addr())
	return uint32(ret), convErr(err)
}

func Version() (uint32, error) {
	if dokanVersion.Find() != nil {
		return 0, errors.New("Failed to load dokan2.dll")
	}
	ret, _, err := syscall.SyscallN(dokanVersion.Addr())
	return uint32(ret), convErr(err)
}

func Init() error {
	if v, err := Version(); err != nil {
		return err
	} else if v < DOKAN_MINIMUM_COMPATIBLE_VERSION {
		return errors.New("Version error")
	}
	_, _, err := syscall.SyscallN(dokanInit.Addr())
	return convErr(err)
}

func Shutdown() error {
	_, _, err := syscall.SyscallN(dokanShutdown.Addr())
	return convErr(err)
}

func MountPoints() (uint32, error) {
	var n uint32
	ret, _, err := syscall.SyscallN(dokanGetMountPointList.Addr(), uintptr(0), uintptr(unsafe.Pointer(&n)))
	if err != 0 {
		return 0, convErr(err)
	}

	syscall.SyscallN(dokanReleaseMountPointList.Addr(), ret)

	return n, convErr(err)
}

func CreateFileSystem(options *DokanOptions, operations *DokanOperations) (uintptr, error) {
	var handle uintptr
	ret, _, err := syscall.SyscallN(dokanCreateFileSystem.Addr(), uintptr(unsafe.Pointer(options)), uintptr(unsafe.Pointer(operations)), uintptr(unsafe.Pointer(&handle)))
	if err != 0 {
		return 0, convErr(err)
	}
	if ret != 0 || handle == 0 {
		return 0, errors.New("Failed to create FS")
	}
	return handle, nil
}

func CloseHandle(handle uintptr) error {
	_, _, err := syscall.SyscallN(dokanCloseHandle.Addr(), handle)
	return convErr(err)
}
