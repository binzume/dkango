package dkango

import (
	"io"
	"io/fs"
	"log"
	"path/filepath"
	"sync"
	"syscall"
	"unsafe"

	"github.com/binzume/dkango/dokan"
)

// You can change this before MountFS() for debugging purpose.
var OptionFlags uint32 = dokan.DOKAN_OPTION_ALT_STREAM // | DOKAN_OPTION_DEBUG | DOKAN_OPTION_STDERR

// UnixTime epoch from 16001-01-01 (UTC) in 0.1us.
const UnixTimeOffset = 116444736000000000

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
	VolumeInfo     VolumeInformation
	TotalBytes     uint64
	AvailableBytes uint64
	WriteProtect   bool // Readonly FS even if fsys implements OpenWriterFS.
}

// TODO: Move this into dokan namespace
type Disk interface {
	GetVolumeInformation(finfo *dokan.FileInfo) (VolumeInformation, dokan.NTStatus)
	GetDiskFreeSpace(availableBytes *uint64, totalBytes *uint64, freeBytes *uint64, finfo *dokan.FileInfo) dokan.NTStatus
	CreateFile(name string, secCtx uintptr, access, attrs, share, disposition, options uint32, finfo *dokan.FileInfo) (FileHandle, dokan.NTStatus)
}

type VolumeInformation struct {
	Name                   string
	SerialNumber           uint32
	MaximumComponentLength uint32
	FileSystemFlags        uint32
	FileSystemName         string
}

type FileHandle interface {
	FindFiles(fillFindDataCallBack func(fi *dokan.WIN32_FIND_DATAW) (int32, syscall.Errno), finfo *dokan.FileInfo) dokan.NTStatus
	GetFileInformation(fi *dokan.ByHandleFileInfo, finfo *dokan.FileInfo) dokan.NTStatus

	ReadFile(buf []byte, read *int32, offset int64, finfo *dokan.FileInfo) dokan.NTStatus
	WriteFile(buf []byte, written *int32, offset int64, finfo *dokan.FileInfo) dokan.NTStatus
	SetEndOfFile(offset int64, finfo *dokan.FileInfo) dokan.NTStatus

	MoveFile(newname string, replaceIfExisting bool, finfo *dokan.FileInfo) dokan.NTStatus
	DeleteFile(finfo *dokan.FileInfo) dokan.NTStatus
	DeleteDir(finfo *dokan.FileInfo) dokan.NTStatus
	Cleanup(finfo *dokan.FileInfo) dokan.NTStatus
	CloseFile(*dokan.FileInfo)
}

type MountInfo struct {
	disk        Disk
	instance    dokan.MountHandle
	openedFiles map[FileHandle]struct{}
	mounted     sync.WaitGroup
	lock        sync.Mutex
	options     *dokan.DokanOptions
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

// Close close MountInfo to unmount this filesystem.
// MountInfo must be closed when it is no longer needed.
func (mi *MountInfo) Close() error {
	err := mi.instance.Close()
	unregisterInstance(mi)
	return err
}

// OpenedFileCount returns number of files currently open in this filesystem.
func (m *MountInfo) OpenedFileCount() int {
	m.lock.Lock()
	defer m.lock.Unlock()
	return len(m.openedFiles)
}

// NotifyCreate notify file create event.
func (m *MountInfo) NotifyCreate(path string, isDir bool) error {
	return dokan.NotifyCreate(m.instance, path, isDir)
}

// NotifyDelete notify file delete event.
func (m *MountInfo) NotifyDelete(path string, isDir bool) error {
	return dokan.NotifyDelete(m.instance, path, isDir)
}

// NotifyRename notify file rename event.
func (m *MountInfo) NotifyRename(oldPath, newPath string, isDir bool) error {
	return dokan.NotifyRename(m.instance, oldPath, newPath, isDir)
}

// NotifyUpdate notify attributes update event.
func (m *MountInfo) NotifyUpdate(path string) error {
	return dokan.NotifyUpdate(m.instance, path)
}

// NotifyXAttrUpdate notify extended attributes update event.
func (m *MountInfo) NotifyXAttrUpdate(path string) error {
	return dokan.NotifyXAttrUpdate(m.instance, path)
}

// keep instances to prevent from GC
var instances = map[*MountInfo]struct{}{}
var instancesLock sync.Mutex
var dokanOperations *dokan.DokanOperations

func registerInstance(mi *MountInfo) error {
	instancesLock.Lock()
	defer instancesLock.Unlock()
	if len(instances) == 0 {
		if err := dokan.Init(); err != nil {
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
		_ = dokan.Shutdown()
	}
}

func ensureDokanOperations() *dokan.DokanOperations {
	instancesLock.Lock()
	defer instancesLock.Unlock()
	if dokanOperations == nil {
		dokanOperations = &dokan.DokanOperations{
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

// MountFS mounts fsys on mountPoint.
func MountFS(mountPoint string, fsys fs.FS, opt *MountOptions) (*MountInfo, error) {
	if opt == nil {
		opt = &MountOptions{
			VolumeInfo:     VolumeInformation{Name: "", FileSystemName: "Dokan"},
			TotalBytes:     1024 * 1024 * 1024,
			AvailableBytes: 1024 * 1024 * 1024,
		}
	}
	d := &disk{
		vi:             opt.VolumeInfo,
		TotalBytes:     opt.TotalBytes,
		AvailableBytes: opt.AvailableBytes,
		fsys:           fsys,
	}
	var ro uint32
	if opt.WriteProtect {
		ro = dokan.DOKAN_OPTION_WRITE_PROTECT
	}
	return MountDisk(mountPoint, d, OptionFlags|ro)
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
	mi.mounted.Add(1)
	path := syscall.StringToUTF16Ptr(mountPoint)
	options := &dokan.DokanOptions{
		Version:       dokan.DOKAN_MINIMUM_COMPATIBLE_VERSION,
		GlobalContext: unsafe.Pointer(mi),
		MountPoint:    uintptr(unsafe.Pointer(path)),
		Options:       optionFlags,
	}

	instance, err := dokan.CreateFileSystem(options, ensureDokanOperations())
	if err != nil {
		unregisterInstance(mi)
		return nil, err
	}
	mi.instance = instance
	mi.options = options
	mi.mounted.Wait()
	return mi, nil
}

func getMountInfo(finfo *dokan.FileInfo) *MountInfo {
	return (*MountInfo)(finfo.DokanOptions.GlobalContext)
}

func getOpenedFile(finfo *dokan.FileInfo) FileHandle {
	return *(*FileHandle)(finfo.Context)
}

func debugCallback() dokan.NTStatus {
	log.Println("debugCallback")
	return dokan.STATUS_NOT_SUPPORTED
}

func getVolumeInformation(pName *uint16, nameSize int32, serial *uint32, maxCLen *uint32, flags *uint32, pSysName *uint16, sysNameSize int32, finfo *dokan.FileInfo) dokan.NTStatus {
	mi := getMountInfo(finfo)
	if mi == nil {
		return dokan.STATUS_INVALID_PARAMETER
	}
	vi, status := mi.disk.GetVolumeInformation(finfo)
	copy(unsafe.Slice(pName, nameSize), syscall.StringToUTF16(vi.Name))
	copy(unsafe.Slice(pSysName, sysNameSize), syscall.StringToUTF16(vi.FileSystemName))
	*serial = vi.SerialNumber
	*maxCLen = vi.MaximumComponentLength
	*flags = vi.FileSystemFlags
	return status
}

func getDiskFreeSpace(availableBytes *uint64, totalBytes *uint64, freeBytes *uint64, finfo *dokan.FileInfo) dokan.NTStatus {
	mi := getMountInfo(finfo)
	if mi == nil {
		return dokan.STATUS_INVALID_PARAMETER
	}
	return mi.disk.GetDiskFreeSpace(availableBytes, totalBytes, freeBytes, finfo)
}

func zwCreateFile(pname *uint16, secCtx uintptr, access, attrs, share, disposition, options uint32, finfo *dokan.FileInfo) dokan.NTStatus {
	mi := getMountInfo(finfo)
	if mi == nil {
		return dokan.STATUS_INVALID_PARAMETER
	}
	f, status := mi.disk.CreateFile(syscall.UTF16ToString(unsafe.Slice(pname, 260)), secCtx, access, attrs, share, disposition, options, finfo)
	if f != nil {
		mi.addFile(f) // avoid GC
		finfo.Context = unsafe.Pointer(&f)
	}
	return status
}

func findFiles(pname *uint16, fillFindData uintptr, finfo *dokan.FileInfo) dokan.NTStatus {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("ERROR: FindFiles: not opened file")
		return dokan.STATUS_INVALID_PARAMETER
	}

	fillFindDataCallBack := func(fi *dokan.WIN32_FIND_DATAW) (int32, syscall.Errno) {
		ret, _, errNo := syscall.SyscallN(fillFindData, uintptr(unsafe.Pointer(&fi)), uintptr(unsafe.Pointer(finfo)))
		return int32(ret), errNo
	}
	return f.FindFiles(fillFindDataCallBack, finfo)
}

func getFileInformation(pname *uint16, fi *dokan.ByHandleFileInfo, finfo *dokan.FileInfo) dokan.NTStatus {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("ERROR: GetFileInformation: not opened file")
		return dokan.STATUS_INVALID_PARAMETER
	}
	return f.GetFileInformation(fi, finfo)
}

func readFile(pname *uint16, buf *byte, sz int32, read *int32, offset int64, finfo *dokan.FileInfo) dokan.NTStatus {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("ERROR: ReadFile: not opened file")
		return dokan.STATUS_INVALID_PARAMETER
	}
	return f.ReadFile(unsafe.Slice(buf, sz), read, offset, finfo)
}

func writeFile(pname *uint16, buf *byte, sz int32, written *int32, offset int64, finfo *dokan.FileInfo) dokan.NTStatus {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("ERROR: WriteFile: not opened file")
		return dokan.STATUS_INVALID_PARAMETER
	}
	return f.WriteFile(unsafe.Slice(buf, sz), written, offset, finfo)
}

func cleanup(pname *uint16, finfo *dokan.FileInfo) dokan.NTStatus {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("ERROR: Cleanup: not opened file")
		return dokan.STATUS_INVALID_PARAMETER
	}
	return f.Cleanup(finfo)
}

func closeFile(pname *uint16, finfo *dokan.FileInfo) uintptr {
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

func deleteFile(pname *uint16, finfo *dokan.FileInfo) dokan.NTStatus {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("ERROR: DeleteFile: not opened file")
		return dokan.STATUS_INVALID_PARAMETER
	}
	return f.DeleteFile(finfo)
}

func deleteDir(pname *uint16, finfo *dokan.FileInfo) dokan.NTStatus {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("ERROR: DeleteDir: not opened file")
		return dokan.STATUS_INVALID_PARAMETER
	}
	return f.DeleteDir(finfo)
}

func moveFile(pname *uint16, pNewName *uint16, replaceIfExisting bool, finfo *dokan.FileInfo) dokan.NTStatus {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("ERROR: MoveFile: not opened?")
		return dokan.STATUS_INVALID_PARAMETER
	}
	return f.MoveFile(syscall.UTF16ToString(unsafe.Slice(pNewName, 260)), replaceIfExisting, finfo)
}

func setEndOfFile(pname *uint16, offset int64, finfo *dokan.FileInfo) dokan.NTStatus {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("ERROR: SetEndOfFile: not opened?")
		return dokan.STATUS_INVALID_PARAMETER
	}
	return f.SetEndOfFile(offset, finfo)
}

func mounted(mountPoint *uint16, finfo *dokan.FileInfo) dokan.NTStatus {
	dk := getMountInfo(finfo)
	if dk == nil {
		return dokan.STATUS_INVALID_PARAMETER
	}
	dk.mounted.Done()
	return dokan.STATUS_SUCCESS
}

func unmounted(finfo *dokan.FileInfo) dokan.NTStatus {
	return dokan.STATUS_SUCCESS
}
