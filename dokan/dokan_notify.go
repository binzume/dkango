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

func NotifyCreate(instance uintptr, filePath string, isDirectory bool) error {
	var dir uintptr
	if isDirectory {
		dir = 1
	}
	u16path, err := syscall.UTF16PtrFromString(filePath)
	if err != nil {
		return err
	}
	ret, _, errNo := syscall.SyscallN(dokanNotifyCreate.Addr(), instance, uintptr(unsafe.Pointer(u16path)), dir)
	if errNo == 0 && ret == 0 {
		return errors.New("Failed to send notification")
	}
	return convErr(errNo)
}

func NotifyDelete(instance uintptr, filePath string, isDirectory bool) error {
	var dir uintptr
	if isDirectory {
		dir = 1
	}
	u16path, err := syscall.UTF16PtrFromString(filePath)
	if err != nil {
		return err
	}
	ret, _, errNo := syscall.SyscallN(dokanNotifyDelete.Addr(), instance, uintptr(unsafe.Pointer(u16path)), dir)
	if errNo == 0 && ret == 0 {
		return errors.New("Failed to send notification")
	}
	return convErr(errNo)
}

func NotifyRename(instance uintptr, oldPath, newPath string, isDirectory bool) error {
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
	ret, _, errNo := syscall.SyscallN(dokanNotifyRename.Addr(), instance, uintptr(unsafe.Pointer(u16old)), uintptr(unsafe.Pointer(u16new)), dir)
	if errNo == 0 && ret == 0 {
		return errors.New("Failed to send notification")
	}
	return convErr(errNo)
}

func NotifyUpdate(instance uintptr, filePath string) error {
	u16path, err := syscall.UTF16PtrFromString(filePath)
	if err != nil {
		return err
	}
	ret, _, errNo := syscall.SyscallN(dokanNotifyUpdate.Addr(), instance, uintptr(unsafe.Pointer(u16path)))
	if errNo == 0 && ret == 0 {
		return errors.New("Failed to send notification")
	}
	return convErr(errNo)
}

func NotifyXAttrUpdate(instance uintptr, filePath string) error {
	u16path, err := syscall.UTF16PtrFromString(filePath)
	if err != nil {
		return err
	}
	ret, _, errNo := syscall.SyscallN(dokanNotifyXAttrUpdate.Addr(), instance, uintptr(unsafe.Pointer(u16path)))
	if errNo == 0 && ret == 0 {
		return errors.New("Failed to send notification")
	}
	return convErr(errNo)
}
