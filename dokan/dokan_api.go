package dokan

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const DOKAN_MINIMUM_COMPATIBLE_VERSION = 200

var (
	dokan2             = windows.NewLazySystemDLL("dokan2.dll")
	dokanDriverVersion = dokan2.NewProc("DokanDriverVersion")
	dokanVersion       = dokan2.NewProc("DokanVersion")
	dokanInit          = dokan2.NewProc("DokanInit")
	dokanShutdown      = dokan2.NewProc("DokanShutdown")

	dokanMain             = dokan2.NewProc("DokanMain")
	dokanCreateFileSystem = dokan2.NewProc("DokanCreateFileSystem")
	dokanCloseHandle      = dokan2.NewProc("DokanCloseHandle")

	dokanWaitForFileSystemClosed = dokan2.NewProc("DokanWaitForFileSystemClosed")
	dokanRemoveMountPoint        = dokan2.NewProc("DokanRemoveMountPoint")
	dokanOpenRequestorToken      = dokan2.NewProc("DokanOpenRequestorToken")

	dokanGetMountPointList     = dokan2.NewProc("DokanGetMountPointList")
	dokanReleaseMountPointList = dokan2.NewProc("DokanReleaseMountPointList")

	dokanResetTimeout = dokan2.NewProc("DokanResetTimeout")
)

func errnoToError(err syscall.Errno) error {
	if err == 0 {
		return nil
	}
	return err
}

func DriverVersion() (uint32, error) {
	if dokanDriverVersion.Find() != nil {
		return 0, ErrFailedToLoadDokan
	}
	ret, _, err := syscall.SyscallN(dokanDriverVersion.Addr())
	return uint32(ret), errnoToError(err)
}

func Version() (uint32, error) {
	if dokanVersion.Find() != nil {
		return 0, ErrFailedToLoadDokan
	}
	ret, _, err := syscall.SyscallN(dokanVersion.Addr())
	return uint32(ret), errnoToError(err)
}

func Init() error {
	if v, err := Version(); err != nil {
		return err
	} else if v < DOKAN_MINIMUM_COMPATIBLE_VERSION {
		return ErrDokanVersion
	}
	_, _, err := syscall.SyscallN(dokanInit.Addr())
	return errnoToError(err)
}

func Shutdown() error {
	_, _, err := syscall.SyscallN(dokanShutdown.Addr())
	return errnoToError(err)
}

func MountPoints() ([]*MountPointInfo, error) {
	var n uint32
	ret, _, err := syscall.SyscallN(dokanGetMountPointList.Addr(), uintptr(0), uintptr(unsafe.Pointer(&n)))
	if err != 0 {
		return nil, errnoToError(err)
	}

	var mps []*MountPointInfo
	// TODO: Fix: possible misuse of unsafe.Pointer
	for _, mp := range unsafe.Slice((*nativeMountPointInfo)(unsafe.Pointer(ret)), n) {
		mps = append(mps, &MountPointInfo{
			Type:         mp.Type,
			MountPoint:   syscall.UTF16ToString(mp.MountPoint[:]),
			UNCName:      syscall.UTF16ToString(mp.UNCName[:]),
			DeviceName:   syscall.UTF16ToString(mp.DeviceName[:]),
			SessionID:    mp.Type,
			MountOptions: mp.MountOptions,
		})
	}

	syscall.SyscallN(dokanReleaseMountPointList.Addr(), ret)
	return mps, errnoToError(err)
}

func (mh MountHandle) Close() error {
	return CloseHandle(uintptr(mh))
}

func CreateFileSystem(options *DokanOptions, operations *DokanOperations) (MountHandle, error) {
	var handle uintptr
	ret, _, err := syscall.SyscallN(dokanCreateFileSystem.Addr(), uintptr(unsafe.Pointer(options)), uintptr(unsafe.Pointer(operations)), uintptr(unsafe.Pointer(&handle)))
	if err != 0 {
		return 0, errnoToError(err)
	}
	if ret != 0 || handle == 0 {
		switch int32(ret) {
		case -1:
			return 0, ErrDokan
		case -2:
			return 0, ErrBadDriveLetter
		case -3:
			return 0, ErrDriverInstall
		case -4:
			return 0, ErrStart
		case -5:
			return 0, ErrMount
		case -6:
			return 0, ErrBadMountPoint
		case -7:
			return 0, ErrDokanVersion
		default:
			return 0, ErrDokan
		}
	}
	return MountHandle(handle), nil
}

func CloseHandle(handle uintptr) error {
	_, _, err := syscall.SyscallN(dokanCloseHandle.Addr(), handle)
	return errnoToError(err)
}

func UTF16FromString(s string) ([]uint16, error) {
	return syscall.UTF16FromString(s)
}

func UTF16PtrFromString(s string) (*uint16, error) {
	return syscall.UTF16PtrFromString(s)
}
