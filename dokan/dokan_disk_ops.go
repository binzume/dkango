//go:build windows
// +build windows

package dokan

import (
	"log"
	"syscall"
	"unsafe"
)

func initDokanOperations() *DokanOperations {
	instancesLock.Lock()
	defer instancesLock.Unlock()
	if dokanOperations == nil {
		dokanOperations = &DokanOperations{
			ZwCreateFile: syscall.NewCallback(zwCreateFile),
			Cleanup:      syscall.NewCallback(cleanup),
			CloseFile:    syscall.NewCallback(closeFile),
			ReadFile:     syscall.NewCallback(readFile),
			WriteFile:    syscall.NewCallback(writeFile),
			// FlushFileBuffers:   debugCallback,
			GetFileInformation: syscall.NewCallback(getFileInformation),
			FindFiles:          syscall.NewCallback(findFiles),
			// FindFilesWithPattern: debugCallback,
			// SetFileAttributes:    debugCallback,
			// SetFileTime:          debugCallback,
			DeleteFile:        syscall.NewCallback(deleteFile),
			DeleteDirectory:   syscall.NewCallback(deleteDir),
			MoveFile:          syscall.NewCallback(moveFile),
			SetEndOfFile:      syscall.NewCallback(setEndOfFile),
			SetAllocationSize: syscall.NewCallback(setEndOfFile),
			// LockFile:             debugCallback,
			// UnlockFile:           debugCallback,
			// GetFileSecurity:      debugCallback,
			// SetFileSecurity:      debugCallback,
			GetDiskFreeSpace:     syscall.NewCallback(getDiskFreeSpace),
			GetVolumeInformation: syscall.NewCallback(getVolumeInformation),
			Mounted:              syscall.NewCallback(mounted),
			Unmounted:            syscall.NewCallback(unmounted),
			// FindStreams:          debugCallback,
		}
	}
	return dokanOperations
}

func getMountInfo(finfo *FileInfo) *MountInfo {
	return (*MountInfo)(finfo.DokanOptions.GlobalContext)
}

func getOpenedFile(finfo *FileInfo) FileHandle {
	return *(*FileHandle)(finfo.Context)
}

func getVolumeInformation(pName *uint16, nameSize int32, serial *uint32, maxCLen *uint32, flags *uint32, pSysName *uint16, sysNameSize int32, finfo *FileInfo) NTStatus {
	mi := getMountInfo(finfo)
	if mi == nil {
		return STATUS_INVALID_PARAMETER
	}
	vi, status := mi.disk.GetVolumeInformation(finfo)
	copy(unsafe.Slice(pName, nameSize), syscall.StringToUTF16(vi.Name))
	copy(unsafe.Slice(pSysName, sysNameSize), syscall.StringToUTF16(vi.FileSystemName))
	*serial = vi.SerialNumber
	*maxCLen = vi.MaximumComponentLength
	*flags = vi.FileSystemFlags
	return status
}

func getDiskFreeSpace(availableBytes *uint64, totalBytes *uint64, freeBytes *uint64, finfo *FileInfo) NTStatus {
	mi := getMountInfo(finfo)
	if mi == nil {
		return STATUS_INVALID_PARAMETER
	}
	return mi.disk.GetDiskFreeSpace(availableBytes, totalBytes, freeBytes, finfo)
}

func zwCreateFile(pname *uint16, secCtx uintptr, access, attrs, share, disposition, options uint32, finfo *FileInfo) NTStatus {
	mi := getMountInfo(finfo)
	if mi == nil {
		return STATUS_INVALID_PARAMETER
	}
	f, status := mi.disk.CreateFile(syscall.UTF16ToString(unsafe.Slice(pname, 260)), secCtx, access, attrs, share, disposition, options, finfo)
	if f != nil {
		mi.addFile(f) // avoid GC
		finfo.Context = unsafe.Pointer(&f)
	}
	return status
}

func findFiles(pname *uint16, fillFindData uintptr, finfo *FileInfo) NTStatus {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("ERROR: FindFiles: not opened file")
		return STATUS_INVALID_PARAMETER
	}

	fillFindDataCallBack := func(fi *WIN32_FIND_DATAW) (bool, error) {
		ret, _, errno := syscall.SyscallN(fillFindData, uintptr(unsafe.Pointer(&fi)), uintptr(unsafe.Pointer(finfo)))
		return ret == 1, errnoToError(errno)
	}
	return f.FindFiles(fillFindDataCallBack, finfo)
}

func getFileInformation(pname *uint16, fi *ByHandleFileInfo, finfo *FileInfo) NTStatus {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("ERROR: GetFileInformation: not opened file")
		return STATUS_INVALID_PARAMETER
	}
	return f.GetFileInformation(fi, finfo)
}

func readFile(pname *uint16, buf *byte, sz int32, read *int32, offset int64, finfo *FileInfo) NTStatus {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("ERROR: ReadFile: not opened file")
		return STATUS_INVALID_PARAMETER
	}
	return f.ReadFile(unsafe.Slice(buf, sz), read, offset, finfo)
}

func writeFile(pname *uint16, buf *byte, sz int32, written *int32, offset int64, finfo *FileInfo) NTStatus {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("ERROR: WriteFile: not opened file")
		return STATUS_INVALID_PARAMETER
	}
	return f.WriteFile(unsafe.Slice(buf, sz), written, offset, finfo)
}

func cleanup(pname *uint16, finfo *FileInfo) NTStatus {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("ERROR: Cleanup: not opened file")
		return STATUS_INVALID_PARAMETER
	}
	return f.Cleanup(finfo)
}

func closeFile(pname *uint16, finfo *FileInfo) uintptr {
	mi := getMountInfo(finfo)
	if mi == nil {
		log.Println("ERROR: CloseFile: no mount info")
		return 0
	}
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("ERROR: CloseFile: not opened file")
		return 0 // CLose() is always succeeded.
	}
	mi.removeFile(f)
	f.CloseFile(finfo)
	finfo.Context = nil
	return 0
}

func deleteFile(pname *uint16, finfo *FileInfo) NTStatus {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("ERROR: DeleteFile: not opened file")
		return STATUS_INVALID_PARAMETER
	}
	return f.DeleteFile(finfo)
}

func deleteDir(pname *uint16, finfo *FileInfo) NTStatus {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("ERROR: DeleteDir: not opened file")
		return STATUS_INVALID_PARAMETER
	}
	return f.DeleteDirectory(finfo)
}

func moveFile(pname *uint16, pNewName *uint16, replaceIfExisting bool, finfo *FileInfo) NTStatus {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("ERROR: MoveFile: not opened?")
		return STATUS_INVALID_PARAMETER
	}
	return f.MoveFile(syscall.UTF16ToString(unsafe.Slice(pNewName, 260)), replaceIfExisting, finfo)
}

func setEndOfFile(pname *uint16, offset int64, finfo *FileInfo) NTStatus {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("ERROR: SetEndOfFile: not opened?")
		return STATUS_INVALID_PARAMETER
	}
	return f.SetEndOfFile(offset, finfo)
}

func mounted(mountPoint *uint16, finfo *FileInfo) NTStatus {
	dk := getMountInfo(finfo)
	if dk == nil {
		return STATUS_INVALID_PARAMETER
	}
	dk.mounted.Done()
	return STATUS_SUCCESS
}

func unmounted(finfo *FileInfo) NTStatus {
	return STATUS_SUCCESS
}
