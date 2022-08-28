package dkango

import (
	"errors"
	"io"
	"io/fs"
	"log"
	"path/filepath"
	"strings"
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
	VolumeName     string
	FileSystemName string
	Serial         uint32
	TotalBytes     uint64
	AvailableBytes uint64
	WriteProtect   bool // Readonly FS even if fsys implements OpenWriterFS.
}

type MountInfo struct {
	fsys        fs.FS
	opt         *MountOptions
	instance    uintptr
	openedFiles map[*openedFile]struct{}
	mounted     sync.WaitGroup
	lock        sync.Mutex
	options     *dokan.DokanOptions
}

func (m *MountInfo) addFile(f *openedFile) {
	m.lock.Lock()
	m.openedFiles[f] = struct{}{}
	m.lock.Unlock()
}

func (m *MountInfo) removeFile(f *openedFile) {
	m.lock.Lock()
	delete(m.openedFiles, f)
	m.lock.Unlock()
}

// Close close MountInfo to unmount this filesystem.
// MountInfo must be closed when it is no longer needed.
func (mi *MountInfo) Close() error {
	err := dokan.CloseHandle(mi.instance)
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

type openedFile struct {
	mi         *MountInfo
	name       string
	openFlag   int
	cachedStat fs.FileInfo
	file       io.Closer
	pos        int64
}

func (f *openedFile) Close() {
	f.mi.removeFile(f)
	f.cachedStat = nil
	if f.file != nil {
		f.file.Close()
	}
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
			VolumeName:     "",
			FileSystemName: "Dokan",
			TotalBytes:     1024 * 1024 * 1024,
			AvailableBytes: 1024 * 1024 * 1024,
		}
	}
	mi := &MountInfo{fsys: fsys, opt: opt, openedFiles: map[*openedFile]struct{}{}}
	if err := registerInstance(mi); err != nil {
		return nil, err
	}
	full, err := filepath.Abs(mountPoint)
	if err == nil {
		mountPoint = full
	}
	var ro uint32
	if opt.WriteProtect {
		ro = dokan.DOKAN_OPTION_WRITE_PROTECT
	}
	mi.mounted.Add(1)
	path := syscall.StringToUTF16Ptr(mountPoint)
	options := &dokan.DokanOptions{
		Version:       205,
		GlobalContext: unsafe.Pointer(mi),
		// SingleThread:  1,
		MountPoint: uintptr(unsafe.Pointer(path)),
		Options:    OptionFlags | ro,
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

func getMountInfo(finfo *dokan.DokanFileInfo) *MountInfo {
	return (*MountInfo)(finfo.DokanOptions.GlobalContext)
}

func getOpenedFile(finfo *dokan.DokanFileInfo) *openedFile {
	return (*openedFile)(finfo.Context)
}

func debugCallback() uintptr {
	log.Println("debugCallback")
	return dokan.STATUS_NOT_SUPPORTED
}

func getVolumeInformation(pName *uint16, nameSize int32, serial *uint32, maxCLen *uint32, flags *uint32, pSysName *uint16, sysNameSize int32, finfo *dokan.DokanFileInfo) uintptr {
	dk := getMountInfo(finfo)
	if dk == nil {
		return dokan.STATUS_INVALID_PARAMETER
	}
	copy(unsafe.Slice(pName, nameSize), syscall.StringToUTF16(dk.opt.VolumeName))
	copy(unsafe.Slice(pSysName, sysNameSize), syscall.StringToUTF16(dk.opt.FileSystemName))
	*serial = dk.opt.Serial
	*maxCLen = 256
	*flags = 0
	return dokan.STATUS_SUCCESS
}

func getDiskFreeSpace(availableBytes *uint64, totalBytes *uint64, freeBytes *uint64, finfo *dokan.DokanFileInfo) uintptr {
	dk := getMountInfo(finfo)
	if dk == nil {
		return dokan.STATUS_INVALID_PARAMETER
	}

	*availableBytes = dk.opt.AvailableBytes
	*totalBytes = dk.opt.TotalBytes
	*freeBytes = dk.opt.AvailableBytes
	return dokan.STATUS_SUCCESS
}

func zwCreateFile(pname *uint16, secCtx uintptr, access, attrs, share, disposition, options uint32, finfo *dokan.DokanFileInfo) uintptr {
	mi := getMountInfo(finfo)
	if mi == nil {
		return dokan.STATUS_INVALID_PARAMETER
	}

	name := syscall.UTF16ToString(unsafe.Slice(pname, 260))
	name = strings.TrimPrefix(filepath.ToSlash(name), "/")
	if name == "" {
		name = "."
	}

	create := disposition == dokan.FILE_CREATE || disposition == dokan.FILE_OPEN_IF || disposition == dokan.FILE_OVERWRITE_IF || disposition == dokan.FILE_SUPERSEDE
	truncate := disposition == dokan.FILE_SUPERSEDE || disposition == dokan.FILE_OVERWRITE || disposition == dokan.FILE_OVERWRITE_IF
	errIfExist := disposition == dokan.FILE_CREATE
	openFlag := 0
	if access&dokan.FILE_WRITE_DATA != 0 && access&dokan.FILE_READ_DATA != 0 {
		openFlag = syscall.O_RDWR
	} else if access&dokan.FILE_READ_DATA != 0 {
		openFlag = syscall.O_RDONLY
	} else if access&dokan.FILE_WRITE_DATA != 0 {
		openFlag = syscall.O_WRONLY
	} else if access&dokan.FILE_APPEND_DATA != 0 {
		openFlag = syscall.O_WRONLY | syscall.O_APPEND
	}

	if openFlag == syscall.O_RDWR || openFlag == syscall.O_WRONLY {
		if create {
			openFlag |= syscall.O_CREAT
		}
		if truncate {
			openFlag |= syscall.O_TRUNC
		}
		if errIfExist {
			openFlag |= syscall.O_EXCL
		}
	}

	stat, err := fs.Stat(mi.fsys, name)
	if err != nil && !(create && errors.Is(err, fs.ErrNotExist)) {
		return dokan.ErrorToNTStatus(err) // Unexpected error
	}
	if err == nil && disposition == dokan.FILE_CREATE {
		return dokan.STATUS_OBJECT_NAME_COLLISION
	}
	if err == nil && stat.IsDir() && options&dokan.FILE_NON_DIRECTORY_FILE != 0 {
		return dokan.STATUS_FILE_IS_A_DIRECTORY
	}
	if err == nil && !stat.IsDir() && options&dokan.FILE_DIRECTORY_FILE != 0 {
		return dokan.STATUS_NOT_A_DIRECTORY
	}

	f := &openedFile{name: name, mi: mi, cachedStat: stat, openFlag: openFlag}

	// Mkdir
	if create && options&dokan.FILE_DIRECTORY_FILE != 0 {
		if fsys, ok := mi.fsys.(MkdirFS); ok {
			err = fsys.Mkdir(name, fs.ModePerm)
			if err != nil {
				return dokan.ErrorToNTStatus(err)
			}
		} else {
			return dokan.STATUS_NOT_SUPPORTED
		}
	}

	// NOTE: Reader is not opened here because sometimes it may only need GetFileInformantion()
	if openFlag != syscall.O_RDONLY && options&dokan.FILE_DIRECTORY_FILE == 0 {
		fsys, ok := f.mi.fsys.(OpenWriterFS)
		if !ok {
			// Readonly FS. TODO: Consider to return STATUS_NOT_SUPPORTED?
			return dokan.STATUS_ACCESS_DENIED
		}
		if truncate {
			f.cachedStat = nil // file size will be cahnged
		}
		w, err := fsys.OpenWriter(name, openFlag)
		if err != nil {
			return dokan.ErrorToNTStatus(err)
		}
		f.file = w
	}

	mi.addFile(f) // avoid GC
	finfo.Context = unsafe.Pointer(f)

	return dokan.STATUS_SUCCESS
}

func findFiles(pname *uint16, fillFindData uintptr, finfo *dokan.DokanFileInfo) uintptr {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("ERROR: FindFiles: not opened file")
		return dokan.STATUS_INVALID_PARAMETER
	}

	proc := func(files []fs.DirEntry) bool {
		for _, file := range files {
			fi := dokan.WIN32_FIND_DATAW{}
			copy(fi.FileName[:], syscall.StringToUTF16(file.Name()))
			if file.IsDir() {
				fi.FileAttributes = dokan.FILE_ATTRIBUTE_DIRECTORY
			} else {
				fi.FileAttributes = dokan.FILE_ATTRIBUTE_NORMAL
			}
			info, err := file.Info()
			if err == nil {
				fi.FileSizeLow = uint32(info.Size())
				fi.FileSizeHigh = uint32(info.Size() >> 32)
				t := (info.ModTime().UnixNano())/100 + UnixTimeOffset
				fi.LastWriteTime[0] = uint32(t)
				fi.LastWriteTime[1] = uint32(t >> 32)
				fi.LastAccessTime = fi.LastWriteTime
				fi.CreationTime = fi.LastWriteTime
			}
			ret, _, errNo := syscall.SyscallN(fillFindData, uintptr(unsafe.Pointer(&fi)), uintptr(unsafe.Pointer(finfo)))
			if errNo != 0 || ret == 1 {
				return false
			}
		}
		return true
	}

	if fsys, ok := f.mi.fsys.(OpenDirFS); ok {
		r, err := fsys.OpenDir(f.name)
		if err != nil {
			return dokan.ErrorToNTStatus(err)
		}
		for {
			if files, _ := r.ReadDir(256); len(files) == 0 || !proc(files) {
				break
			}
		}
	} else {
		files, err := fs.ReadDir(f.mi.fsys, f.name)
		proc(files)
		return dokan.ErrorToNTStatus(err)
	}
	return dokan.STATUS_SUCCESS
}

func getFileInformation(pname *uint16, fi *dokan.ByHandleFileInfo, finfo *dokan.DokanFileInfo) uintptr {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("ERROR: GetFileInformation: not opened file")
		return dokan.STATUS_INVALID_PARAMETER
	}

	if f.cachedStat == nil {
		stat, err := fs.Stat(f.mi.fsys, f.name)
		f.cachedStat = stat
		if err != nil {
			return dokan.ErrorToNTStatus(err)
		}
	}
	if f.cachedStat.IsDir() {
		fi.FileAttributes = dokan.FILE_ATTRIBUTE_DIRECTORY
	}
	if f.cachedStat.Mode()&0o200 == 0 {
		fi.FileAttributes = dokan.FILE_ATTRIBUTE_READONLY
	}
	if fi.FileAttributes == 0 {
		fi.FileAttributes = dokan.FILE_ATTRIBUTE_NORMAL
	}
	fi.FileSizeLow = uint32(f.cachedStat.Size())
	fi.FileSizeHigh = uint32(f.cachedStat.Size() >> 32)
	t := (f.cachedStat.ModTime().UnixNano())/100 + UnixTimeOffset
	fi.LastWriteTime[0] = uint32(t)
	fi.LastWriteTime[1] = uint32(t >> 32)
	fi.LastAccessTime = fi.LastWriteTime
	fi.CreationTime = fi.LastWriteTime
	fi.VolumeSerialNumber = int32(f.mi.opt.Serial)

	return dokan.STATUS_SUCCESS
}

func readFile(pname *uint16, buf *byte, sz int32, read *int32, offset int64, finfo *dokan.DokanFileInfo) uintptr {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("ERROR: ReadFile: not opened file")
		return dokan.STATUS_INVALID_PARAMETER
	}

	if f.file == nil {
		r, err := f.mi.fsys.Open(f.name)
		if err != nil {
			return dokan.ErrorToNTStatus(err)
		}
		f.file = r
	}
	if f.pos != offset {
		if seeker, ok := f.file.(io.Seeker); ok {
			_, err := seeker.Seek(offset, io.SeekStart)
			if err != nil {
				return dokan.ErrorToNTStatus(err)
			}
		} else if r, ok := f.file.(io.ReaderAt); ok {
			n, err := r.ReadAt(unsafe.Slice(buf, sz), offset)
			f.pos = -1
			*read = int32(n)
			if n > 0 {
				return dokan.STATUS_SUCCESS // ignore EOF error
			}
			return dokan.ErrorToNTStatus(err)
		} else {
			return dokan.STATUS_NOT_SUPPORTED
		}
	}
	if r, ok := f.file.(io.Reader); ok {
		n, err := r.Read(unsafe.Slice(buf, sz))
		f.pos = offset + int64(n)
		*read = int32(n)
		if n > 0 {
			return dokan.STATUS_SUCCESS // ignore EOF error
		}
		return dokan.ErrorToNTStatus(err)
	}
	return dokan.STATUS_NOT_SUPPORTED
}

func writeFile(pname *uint16, buf *byte, sz int32, written *int32, offset int64, finfo *dokan.DokanFileInfo) uintptr {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("ERROR: WriteFile: not opened file")
		return dokan.STATUS_INVALID_PARAMETER
	}

	if f.openFlag == syscall.O_RDONLY || f.file == nil {
		return dokan.STATUS_ACCESS_DENIED
	}

	// TODO: handle negative offset correctly.
	if offset >= 0 && f.pos != offset {
		if seeker, ok := f.file.(io.Seeker); ok {
			_, err := seeker.Seek(offset, io.SeekStart)
			if err != nil {
				return dokan.ErrorToNTStatus(err)
			}
		} else if w, ok := f.file.(io.WriterAt); ok {
			f.cachedStat = nil // invalidate cached stat
			f.pos = -1         // TODO
			n, err := w.WriteAt(unsafe.Slice(buf, sz), offset)
			*written = int32(n)
			return dokan.ErrorToNTStatus(err)
		} else {
			return dokan.STATUS_NOT_SUPPORTED
		}
	}
	if r, ok := f.file.(io.Writer); ok {
		n, err := r.Write(unsafe.Slice(buf, sz))
		f.pos = offset + int64(n)
		*written = int32(n)
		return dokan.ErrorToNTStatus(err)
	}
	return dokan.STATUS_NOT_SUPPORTED
}

func cleanup(pname *uint16, finfo *dokan.DokanFileInfo) uintptr {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("ERROR: Cleanup: not opened file")
		return dokan.STATUS_INVALID_PARAMETER
	}

	if finfo.DeleteOnClose == 0 {
		return dokan.STATUS_SUCCESS
	}
	if fsys, ok := f.mi.fsys.(RemoveFS); ok {
		return dokan.ErrorToNTStatus(fsys.Remove(f.name))
	}
	return dokan.STATUS_NOT_SUPPORTED
}

func closeFile(pname *uint16, finfo *dokan.DokanFileInfo) uintptr {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("ERROR: CloseFile: not opened file")
		return dokan.STATUS_INVALID_PARAMETER
	}
	f.Close()
	finfo.Context = nil
	return dokan.STATUS_SUCCESS
}

func deleteFile(pname *uint16, finfo *dokan.DokanFileInfo) uintptr {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("ERROR: DeleteFile: not opened file")
		return dokan.STATUS_INVALID_PARAMETER
	}
	return dokan.STATUS_SUCCESS
}

func deleteDir(pname *uint16, finfo *dokan.DokanFileInfo) uintptr {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("ERROR: DeleteDir: not opened file")
		return dokan.STATUS_INVALID_PARAMETER
	}
	return dokan.STATUS_SUCCESS
}

func moveFile(pname *uint16, pNewName *uint16, replaceIfExisting bool, finfo *dokan.DokanFileInfo) uintptr {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("ERROR: MoveFile: not opened?")
		return dokan.STATUS_INVALID_PARAMETER
	}

	fsys, ok := f.mi.fsys.(RenameFS)
	if !ok {
		log.Println("WARN: MoveFile: not support Rename()")
		return dokan.STATUS_NOT_SUPPORTED
	}

	name := syscall.UTF16ToString(unsafe.Slice(pNewName, 260))
	name = strings.TrimPrefix(filepath.ToSlash(name), "/")
	if name == "" {
		name = "."
	}
	f.cachedStat = nil
	return dokan.ErrorToNTStatus(fsys.Rename(f.name, name))
}

func setEndOfFile(pname *uint16, offset int64, finfo *dokan.DokanFileInfo) uintptr {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("ERROR: SetEndOfFile: not opened?")
		return dokan.STATUS_INVALID_PARAMETER
	}

	if trunc, ok := f.file.(interface{ Truncate(int64) error }); ok {
		f.cachedStat = nil
		return dokan.ErrorToNTStatus(trunc.Truncate(offset))
	} else if fsys, ok := f.mi.fsys.(TruncateFS); ok {
		f.cachedStat = nil
		return dokan.ErrorToNTStatus(fsys.Truncate(f.name, offset))
	}
	return dokan.STATUS_NOT_SUPPORTED
}

func mounted(mountPoint *uint16, finfo *dokan.DokanFileInfo) uintptr {
	dk := getMountInfo(finfo)
	if dk == nil {
		return dokan.STATUS_INVALID_PARAMETER
	}
	dk.mounted.Done()
	return dokan.STATUS_SUCCESS
}

func unmounted(finfo *dokan.DokanFileInfo) uintptr {
	return dokan.STATUS_SUCCESS
}
