package dokan

import (
	"log"
	"path/filepath"
	"sync"
	"syscall"
	"unsafe"
)

type Disk interface {
	GetVolumeInformation(finfo *FileInfo) (VolumeInformation, NTStatus)
	GetDiskFreeSpace(availableBytes *uint64, totalBytes *uint64, freeBytes *uint64, finfo *FileInfo) NTStatus
	CreateFile(name string, secCtx uintptr, access, attrs, share, disposition, options uint32, finfo *FileInfo) (FileHandle, NTStatus)
}

type VolumeInformation struct {
	Name                   string
	SerialNumber           uint32
	MaximumComponentLength uint32
	FileSystemFlags        uint32
	FileSystemName         string
}

type FileHandle interface {
	FindFiles(fillFindDataCallBack func(fi *WIN32_FIND_DATAW) (int32, syscall.Errno), finfo *FileInfo) NTStatus
	GetFileInformation(fi *ByHandleFileInfo, finfo *FileInfo) NTStatus

	ReadFile(buf []byte, read *int32, offset int64, finfo *FileInfo) NTStatus
	WriteFile(buf []byte, written *int32, offset int64, finfo *FileInfo) NTStatus
	SetEndOfFile(offset int64, finfo *FileInfo) NTStatus

	MoveFile(newname string, replaceIfExisting bool, finfo *FileInfo) NTStatus
	DeleteFile(finfo *FileInfo) NTStatus
	DeleteDirectory(finfo *FileInfo) NTStatus
	Cleanup(finfo *FileInfo) NTStatus
	CloseFile(*FileInfo)
}

type MountInfo struct {
	disk        Disk
	instance    MountHandle
	openedFiles map[FileHandle]struct{}
	mounted     sync.WaitGroup
	lock        sync.Mutex
	options     *DokanOptions
}

func (m *MountInfo) addFile(f FileHandle) {
	m.lock.Lock()
	m.openedFiles[f] = struct{}{}
	m.lock.Unlock()
}

func (m *MountInfo) removeFile(f FileHandle) {
	m.lock.Lock()
	delete(m.openedFiles, f)
	m.lock.Unlock()
}

func (m *MountInfo) Disk() Disk {
	return m.disk
}

// OpenedFileCount returns number of files currently open in this filesystem.
func (m *MountInfo) OpenedFileCount() int {
	m.lock.Lock()
	defer m.lock.Unlock()
	return len(m.openedFiles)
}

// Close close MountInfo to unmount this filesystem.
// MountInfo must be closed when it is no longer needed.
func (mi *MountInfo) Close() error {
	err := mi.instance.Close()
	unregisterInstance(mi)
	return err
}

// NotifyCreate notify file create event.
func (m *MountInfo) NotifyCreate(path string, isDir bool) error {
	return NotifyCreate(m.instance, path, isDir)
}

// NotifyDelete notify file delete event.
func (m *MountInfo) NotifyDelete(path string, isDir bool) error {
	return NotifyDelete(m.instance, path, isDir)
}

// NotifyRename notify file rename event.
func (m *MountInfo) NotifyRename(oldPath, newPath string, isDir bool) error {
	return NotifyRename(m.instance, oldPath, newPath, isDir)
}

// NotifyUpdate notify attributes update event.
func (m *MountInfo) NotifyUpdate(path string) error {
	return NotifyUpdate(m.instance, path)
}

// NotifyXAttrUpdate notify extended attributes update event.
func (m *MountInfo) NotifyXAttrUpdate(path string) error {
	return NotifyXAttrUpdate(m.instance, path)
}

// keep instances to prevent from GC
var instances = map[*MountInfo]struct{}{}
var instancesLock sync.Mutex
var dokanOperations *DokanOperations

func registerInstance(mi *MountInfo) error {
	instancesLock.Lock()
	defer instancesLock.Unlock()
	if len(instances) == 0 {
		if err := Init(); err != nil {
			return err
		}
	}
	instances[mi] = struct{}{}
	return nil
}

func unregisterInstance(mi *MountInfo) {
	instancesLock.Lock()
	defer instancesLock.Unlock()
	delete(instances, mi)
	if len(instances) == 0 {
		_ = Shutdown()
	}
}

func ensureDokanOperations() *DokanOperations {
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

func MountDisk(mountPoint string, d Disk, optionFlags uint32) (*MountInfo, error) {
	mi := &MountInfo{disk: d, openedFiles: map[FileHandle]struct{}{}}
	if err := registerInstance(mi); err != nil {
		return nil, err
	}
	full, err := filepath.Abs(mountPoint)
	if err == nil {
		mountPoint = full
	}
	path := syscall.StringToUTF16Ptr(mountPoint)
	options := &DokanOptions{
		Version:       DOKAN_MINIMUM_COMPATIBLE_VERSION,
		GlobalContext: unsafe.Pointer(mi),
		MountPoint:    unsafe.Pointer(path),
		Options:       optionFlags,
	}

	mi.mounted.Add(1)
	instance, err := CreateFileSystem(options, ensureDokanOperations())
	if err != nil {
		unregisterInstance(mi)
		return nil, err
	}
	mi.instance = instance
	mi.options = options
	mi.mounted.Wait()
	return mi, nil
}

func getMountInfo(finfo *FileInfo) *MountInfo {
	return (*MountInfo)(finfo.DokanOptions.GlobalContext)
}

func getOpenedFile(finfo *FileInfo) FileHandle {
	return *(*FileHandle)(finfo.Context)
}

func debugCallback() NTStatus {
	log.Println("debugCallback")
	return STATUS_NOT_SUPPORTED
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

	fillFindDataCallBack := func(fi *WIN32_FIND_DATAW) (int32, syscall.Errno) {
		ret, _, errNo := syscall.SyscallN(fillFindData, uintptr(unsafe.Pointer(&fi)), uintptr(unsafe.Pointer(finfo)))
		return int32(ret), errNo
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
