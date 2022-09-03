package dokan

import (
	"path/filepath"
	"sync"
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
	FindFiles(fillFindDataCallBack func(fi *WIN32_FIND_DATAW) (buffferFull bool, err error), finfo *FileInfo) NTStatus
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
	openedFiles map[unsafe.Pointer]struct{}
	mounted     sync.WaitGroup
	lock        sync.Mutex
	options     *DokanOptions
}

func (m *MountInfo) addFile(f unsafe.Pointer) {
	m.lock.Lock()
	m.openedFiles[f] = struct{}{}
	m.lock.Unlock()
}

func (m *MountInfo) removeFile(f unsafe.Pointer) {
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

func MountDisk(mountPoint string, d Disk, optionFlags uint32) (*MountInfo, error) {
	mi := &MountInfo{disk: d, openedFiles: map[unsafe.Pointer]struct{}{}}
	if err := registerInstance(mi); err != nil {
		return nil, err
	}
	full, err := filepath.Abs(mountPoint)
	if err == nil {
		mountPoint = full
	}
	path, err := UTF16PtrFromString(mountPoint)
	if err != nil {
		return nil, err
	}
	options := &DokanOptions{
		Version:       DOKAN_MINIMUM_COMPATIBLE_VERSION,
		GlobalContext: unsafe.Pointer(mi),
		MountPoint:    unsafe.Pointer(path),
		Options:       optionFlags,
	}

	mi.mounted.Add(1)
	instance, err := CreateFileSystem(options, initDokanOperations())
	if err != nil {
		unregisterInstance(mi)
		return nil, err
	}
	mi.instance = instance
	mi.options = options
	mi.mounted.Wait()
	return mi, nil
}
