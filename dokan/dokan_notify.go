//go:build windows
// +build windows

package dokan

import (
	"errors"
	"syscall"
	"unsafe"
)

var (
	// Notify
	dokanNotifyCreate      = dokan2.NewProc("DokanNotifyCreate")
	dokanNotifyDelete      = dokan2.NewProc("DokanNotifyDelete")
	dokanNotifyRename      = dokan2.NewProc("DokanNotifyRename")
	dokanNotifyUpdate      = dokan2.NewProc("DokanNotifyUpdate")
	dokanNotifyXAttrUpdate = dokan2.NewProc("DokanNotifyXAttrUpdate")
)

func NotifyCreate(instance MountHandle, filePath string, isDirectory bool) error {
	var dir uintptr
	if isDirectory {
		dir = 1
	}
	u16path, err := syscall.UTF16PtrFromString(filePath)
	if err != nil {
		return err
	}
	ret, _, errNo := syscall.SyscallN(dokanNotifyCreate.Addr(), uintptr(instance), uintptr(unsafe.Pointer(u16path)), dir)
	if errNo == 0 && ret == 0 {
		return errors.New("Failed to send notification")
	}
	return errnoToError(errNo)
}

func NotifyDelete(instance MountHandle, filePath string, isDirectory bool) error {
	var dir uintptr
	if isDirectory {
		dir = 1
	}
	u16path, err := syscall.UTF16PtrFromString(filePath)
	if err != nil {
		return err
	}
	ret, _, errNo := syscall.SyscallN(dokanNotifyDelete.Addr(), uintptr(instance), uintptr(unsafe.Pointer(u16path)), dir)
	if errNo == 0 && ret == 0 {
		return errors.New("Failed to send notification")
	}
	return errnoToError(errNo)
}

func NotifyRename(instance MountHandle, oldPath, newPath string, isDirectory bool) error {
	var dir uintptr
	if isDirectory {
		dir = 1
	}
	u16old, err := syscall.UTF16PtrFromString(oldPath)
	if err != nil {
		return err
	}
	u16new, err := syscall.UTF16PtrFromString(newPath)
	if err != nil {
		return err
	}
	ret, _, errNo := syscall.SyscallN(dokanNotifyRename.Addr(), uintptr(instance), uintptr(unsafe.Pointer(u16old)), uintptr(unsafe.Pointer(u16new)), dir)
	if errNo == 0 && ret == 0 {
		return errors.New("Failed to send notification")
	}
	return errnoToError(errNo)
}

func NotifyUpdate(instance MountHandle, filePath string) error {
	u16path, err := syscall.UTF16PtrFromString(filePath)
	if err != nil {
		return err
	}
	ret, _, errNo := syscall.SyscallN(dokanNotifyUpdate.Addr(), uintptr(instance), uintptr(unsafe.Pointer(u16path)))
	if errNo == 0 && ret == 0 {
		return errors.New("Failed to send notification")
	}
	return errnoToError(errNo)
}

func NotifyXAttrUpdate(instance MountHandle, filePath string) error {
	u16path, err := syscall.UTF16PtrFromString(filePath)
	if err != nil {
		return err
	}
	ret, _, errNo := syscall.SyscallN(dokanNotifyXAttrUpdate.Addr(), uintptr(instance), uintptr(unsafe.Pointer(u16path)))
	if errNo == 0 && ret == 0 {
		return errors.New("Failed to send notification")
	}
	return errnoToError(errNo)
}
