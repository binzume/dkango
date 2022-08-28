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
)

// You can change this before MountFS() for debugging purpose.
var OptionFlags uint32 = DOKAN_OPTION_ALT_STREAM // | DOKAN_OPTION_DEBUG | DOKAN_OPTION_STDERR

// UnixTime epoch from 16001-01-01 (UTC) in 0.1us.
const UnixTimeOffset = 116444736000000000

type OpenWriterFS interface {
	fs.FS
	OpenWriter(name string, flag int) (io.WriteCloser, error)
}

type RemoveFS interface {
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
}

type MountInfo struct {
	fsys        fs.FS
	opt         *MountOptions
	handle      uintptr
	openedFiles map[*openedFile]struct{}
	mounted     sync.WaitGroup
	lock        sync.Mutex
	operations  *DokanOperations
	options     *DokanOptions
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

func (m *MountInfo) OpenedFileCount() int {
	m.lock.Lock()
	defer m.lock.Unlock()
	return len(m.openedFiles)
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

// MountFS mount fsys in mountPoint.
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
	mi.mounted.Add(1)
	path := syscall.StringToUTF16Ptr(mountPoint)
	options := &DokanOptions{
		Version:       205,
		GlobalContext: unsafe.Pointer(mi),
		// SingleThread:  1,
		MountPoint: uintptr(unsafe.Pointer(path)),
		Options:    OptionFlags,
	}
	operations := &DokanOperations{
		ZwCreateFile: zwCreateFile,
		Cleanup:      cleanup,
		CloseFile:    closeFile,
		ReadFile:     readFile,
		WriteFile:    writeFile,

		// FlushFileBuffers:   debugCallback,
		GetFileInformation: getFileInformation,
		FindFiles:          syscall.NewCallback(findFiles),
		// FindFilesWithPattern: syscall.NewCallback(findFilesWithPattern),
		// SetFileAttributes:    debugCallback,
		// SetFileTime:          debugCallback,
		DeleteFile:        deleteFile,
		DeleteDirectory:   deleteDir,
		MoveFile:          moveFile,
		SetEndOfFile:      setEndOfFile,
		SetAllocationSize: setEndOfFile,
		// LockFile:             debugCallback,
		// UnlockFile:           debugCallback,
		// GetFileSecurity:      debugCallback,
		// SetFileSecurity:      debugCallback,
		GetDiskFreeSpace:     getDiskFreeSpace,
		GetVolumeInformation: getVolumeInformation,
		Unmounted:            unmounted,
		// FindStreams:          debugCallback,
		Mounted: mounted,
	}

	handle, err := CreateFileSystem(options, operations)
	if err != nil {
		unregisterInstance(mi)
		return nil, err
	}
	mi.handle = handle
	mi.options = options
	mi.operations = operations
	mi.mounted.Wait()
	return mi, nil
}

func (mi *MountInfo) Close() error {
	err := CloseHandle(mi.handle)
	unregisterInstance(mi)
	return err
}

func getMountInfo(finfo *DokanFileInfo) *MountInfo {
	return (*MountInfo)(finfo.DokanOptions.GlobalContext)
}

func getOpenedFile(finfo *DokanFileInfo) *openedFile {
	return (*openedFile)(finfo.Context)
}

func errToStatus(err error) uintptr {
	if err == nil {
		return STATUS_SUCCESS
	} else if errors.Is(err, fs.ErrNotExist) {
		return STATUS_OBJECT_NAME_NOT_FOUND
	} else if err == io.EOF || errors.Is(err, io.ErrUnexpectedEOF) {
		return STATUS_END_OF_FILE
	}
	return STATUS_ACCESS_DENIED
}

var debugCallback = syscall.NewCallback(func(param uintptr) uintptr {
	log.Println("debugCallback", param)
	return STATUS_ACCESS_DENIED
})

var getVolumeInformation = syscall.NewCallback(func(pName *uint16, nameSize int32, serial *uint32, maxCLen *uint32, flags *uint32, pSysName *uint16, sysNameSize int32, finfo *DokanFileInfo) uintptr {
	dk := getMountInfo(finfo)
	if dk == nil {
		return STATUS_ACCESS_DENIED
	}
	copy(unsafe.Slice(pName, nameSize), syscall.StringToUTF16(dk.opt.VolumeName))
	copy(unsafe.Slice(pSysName, sysNameSize), syscall.StringToUTF16(dk.opt.FileSystemName))
	*serial = dk.opt.Serial
	*maxCLen = 256
	*flags = 0
	return STATUS_SUCCESS
})

var getDiskFreeSpace = syscall.NewCallback(func(availableBytes *uint64, totalBytes *uint64, freeBytes *uint64, finfo *DokanFileInfo) uintptr {
	dk := getMountInfo(finfo)
	if dk == nil {
		return STATUS_INVALID_PARAMETER
	}

	*availableBytes = dk.opt.AvailableBytes
	*totalBytes = dk.opt.TotalBytes
	*freeBytes = dk.opt.AvailableBytes
	return STATUS_SUCCESS
})

var mounted = syscall.NewCallback(func(mountPoint *uint16, finfo *DokanFileInfo) uintptr {
	dk := getMountInfo(finfo)
	if dk == nil {
		return STATUS_INVALID_PARAMETER
	}
	dk.mounted.Done()
	return STATUS_SUCCESS
})

var unmounted = syscall.NewCallback(func(finfo *DokanFileInfo) uintptr {
	return STATUS_SUCCESS
})

var zwCreateFile = syscall.NewCallback(func(pname *uint16, secCtx uintptr, access, attrs, share, disposition, options uint32, finfo *DokanFileInfo) uintptr {
	mi := getMountInfo(finfo)
	if mi == nil {
		return STATUS_INVALID_PARAMETER
	}

	name := syscall.UTF16ToString(unsafe.Slice(pname, 260))
	name = strings.TrimPrefix(filepath.ToSlash(name), "/")
	if name == "" {
		name = "."
	}

	create := disposition == FILE_CREATE || disposition == FILE_OPEN_IF || disposition == FILE_OVERWRITE_IF || disposition == FILE_SUPERSEDE
	truncate := disposition == FILE_SUPERSEDE || disposition == FILE_OVERWRITE || disposition == FILE_OVERWRITE_IF
	errIfExist := disposition == FILE_CREATE
	openFlag := 0
	if access&FILE_WRITE_DATA != 0 && access&FILE_READ_DATA != 0 {
		openFlag = syscall.O_RDWR
	} else if access&FILE_READ_DATA != 0 {
		openFlag = syscall.O_RDONLY
	} else if access&FILE_WRITE_DATA != 0 {
		openFlag = syscall.O_WRONLY
	} else if access&FILE_APPEND_DATA != 0 {
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
		return errToStatus(err) // Unexpected error
	}
	if err == nil && disposition == FILE_CREATE {
		return STATUS_OBJECT_NAME_COLLISION
	}
	if err == nil && stat.IsDir() && options&FILE_NON_DIRECTORY_FILE != 0 {
		return STATUS_FILE_IS_A_DIRECTORY
	}
	if err == nil && !stat.IsDir() && options&FILE_DIRECTORY_FILE != 0 {
		return STATUS_NOT_A_DIRECTORY
	}

	f := &openedFile{name: name, mi: mi, cachedStat: stat, openFlag: openFlag}

	// Mkdir
	if create && options&FILE_DIRECTORY_FILE != 0 {
		if fsys, ok := mi.fsys.(MkdirFS); ok {
			err = fsys.Mkdir(name, fs.ModePerm)
			if err != nil {
				return errToStatus(err)
			}
		} else {
			return STATUS_NOT_SUPPORTED
		}
	}

	// NOTE: Reader is not opened here because sometimes it may only need GetFileInformantion()
	if openFlag != syscall.O_RDONLY && options&FILE_DIRECTORY_FILE == 0 {
		fsys, ok := f.mi.fsys.(OpenWriterFS)
		if !ok {
			// Readonly FS. TODO: Consider to return STATUS_NOT_SUPPORTED?
			return STATUS_ACCESS_DENIED
		}
		if truncate {
			f.cachedStat = nil // file size will be cahnged
		}
		w, err := fsys.OpenWriter(name, openFlag)
		if err != nil {
			return errToStatus(err)
		}
		f.file = w
	}

	mi.addFile(f) // avoid GC
	finfo.Context = unsafe.Pointer(f)

	return STATUS_SUCCESS
})

func findFiles(pname *uint16, fillFindData uintptr, finfo *DokanFileInfo) uintptr {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("FindFIles not opened?")
		return STATUS_INVALID_PARAMETER
	}
	if f.name == "" {
		f.name = "."
	}

	proc := func(files []fs.DirEntry) bool {
		for _, file := range files {
			fi := WIN32_FIND_DATAW{}
			copy(fi.FileName[:], syscall.StringToUTF16(file.Name()))
			if file.IsDir() {
				fi.FileAttributes = FILE_ATTRIBUTE_DIRECTORY
			} else {
				fi.FileAttributes = FILE_ATTRIBUTE_NORMAL
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
			return errToStatus(err)
		}
		for {
			if files, _ := r.ReadDir(256); len(files) == 0 || !proc(files) {
				break
			}
		}
	} else {
		files, err := fs.ReadDir(f.mi.fsys, f.name)
		proc(files)
		return errToStatus(err)
	}
	return STATUS_SUCCESS
}

func findFilesWithPattern(pname *uint16, pattern *uint16, fillFindData uintptr, finfo *DokanFileInfo) uintptr {
	// TODO: pattern
	return findFiles(pname, fillFindData, finfo)
}

var getFileInformation = syscall.NewCallback(func(pname *uint16, fi *ByHandleFileInfo, finfo *DokanFileInfo) uintptr {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("GetFileInfo: not opened?")
		return STATUS_INVALID_PARAMETER
	}

	if f.cachedStat == nil {
		stat, err := fs.Stat(f.mi.fsys, f.name)
		f.cachedStat = stat
		if err != nil {
			return errToStatus(err)
		}
	}
	if f.cachedStat.IsDir() {
		fi.FileAttributes = FILE_ATTRIBUTE_DIRECTORY
	}
	if f.cachedStat.Mode()&0o200 == 0 {
		fi.FileAttributes = FILE_ATTRIBUTE_READONLY
	}
	if fi.FileAttributes == 0 {
		fi.FileAttributes = FILE_ATTRIBUTE_NORMAL
	}
	fi.FileSizeLow = uint32(f.cachedStat.Size())
	fi.FileSizeHigh = uint32(f.cachedStat.Size() >> 32)
	t := (f.cachedStat.ModTime().UnixNano())/100 + UnixTimeOffset
	fi.LastWriteTime[0] = uint32(t)
	fi.LastWriteTime[1] = uint32(t >> 32)
	fi.LastAccessTime = fi.LastWriteTime
	fi.CreationTime = fi.LastWriteTime
	fi.VolumeSerialNumber = int32(f.mi.opt.Serial)

	return STATUS_SUCCESS
})

var readFile = syscall.NewCallback(func(pname *uint16, buf *byte, sz int32, read *int32, offset int64, finfo *DokanFileInfo) uintptr {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("ReadFile: not opened?")
		return STATUS_INVALID_PARAMETER
	}

	if f.file == nil {
		r, err := f.mi.fsys.Open(f.name)
		if err != nil {
			return errToStatus(err)
		}
		f.file = r
	}
	if f.pos != offset {
		if seeker, ok := f.file.(io.Seeker); ok {
			_, err := seeker.Seek(offset, io.SeekStart)
			if err != nil {
				return errToStatus(err)
			}
		} else if r, ok := f.file.(io.ReaderAt); ok {
			n, err := r.ReadAt(unsafe.Slice(buf, sz), offset)
			f.pos = -1
			*read = int32(n)
			if n > 0 {
				return STATUS_SUCCESS // ignore EOF error
			}
			return errToStatus(err)
		} else {
			return STATUS_NOT_SUPPORTED
		}
	}
	if r, ok := f.file.(io.Reader); ok {
		n, err := r.Read(unsafe.Slice(buf, sz))
		f.pos = offset + int64(n)
		*read = int32(n)
		if n > 0 {
			return STATUS_SUCCESS // ignore EOF error
		}
		return errToStatus(err)
	}
	return STATUS_NOT_SUPPORTED
})

var writeFile = syscall.NewCallback(func(pname *uint16, buf *byte, sz int32, written *int32, offset int64, finfo *DokanFileInfo) uintptr {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("WriteFile: not opened?")
		return STATUS_INVALID_PARAMETER
	}

	if f.openFlag == syscall.O_RDONLY || f.file == nil {
		return STATUS_ACCESS_DENIED
	}

	// TODO: handle negative offset correctly.
	if offset >= 0 && f.pos != offset {
		if seeker, ok := f.file.(io.Seeker); ok {
			_, err := seeker.Seek(offset, io.SeekStart)
			if err != nil {
				return errToStatus(err)
			}
		} else if w, ok := f.file.(io.WriterAt); ok {
			f.cachedStat = nil // invalidate cached stat
			f.pos = -1         // TODO
			n, err := w.WriteAt(unsafe.Slice(buf, sz), offset)
			*written = int32(n)
			return errToStatus(err)
		} else {
			return STATUS_NOT_SUPPORTED
		}
	}
	if r, ok := f.file.(io.Writer); ok {
		n, err := r.Write(unsafe.Slice(buf, sz))
		f.pos = offset + int64(n)
		*written = int32(n)
		return errToStatus(err)
	}
	return STATUS_NOT_SUPPORTED
})

var cleanup = syscall.NewCallback(func(pname *uint16, finfo *DokanFileInfo) uintptr {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("CloseFile: not opened?")
		return STATUS_INVALID_PARAMETER
	}

	if finfo.DeleteOnClose == 0 {
		return STATUS_SUCCESS
	}
	if fsys, ok := f.mi.fsys.(RemoveFS); ok {
		return errToStatus(fsys.Remove(f.name))
	}
	return STATUS_NOT_SUPPORTED
})

var closeFile = syscall.NewCallback(func(pname *uint16, finfo *DokanFileInfo) uintptr {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("CloseFile: not opened?")
		return STATUS_INVALID_PARAMETER
	}
	f.Close()
	finfo.Context = nil
	return STATUS_SUCCESS
})

var deleteFile = syscall.NewCallback(func(pname *uint16, finfo *DokanFileInfo) uintptr {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("DeleteFile: not opened?")
		return STATUS_INVALID_PARAMETER
	}
	return STATUS_SUCCESS
})

var deleteDir = syscall.NewCallback(func(pname *uint16, finfo *DokanFileInfo) uintptr {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("DeleteDir: not opened?")
		return STATUS_INVALID_PARAMETER
	}
	return STATUS_SUCCESS
})

var moveFile = syscall.NewCallback(func(pname *uint16, pNewName *uint16, replaceIfExisting bool, finfo *DokanFileInfo) uintptr {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("MoveFile: not opened?")
		return STATUS_INVALID_PARAMETER
	}

	fsys, ok := f.mi.fsys.(RenameFS)
	if !ok {
		log.Println("MoveFIle: not support Rename()?")
		return STATUS_NOT_SUPPORTED
	}

	name := syscall.UTF16ToString(unsafe.Slice(pNewName, 260))
	name = strings.TrimPrefix(filepath.ToSlash(name), "/")
	if name == "" {
		name = "."
	}
	f.cachedStat = nil
	return errToStatus(fsys.Rename(f.name, name))
})

var setEndOfFile = syscall.NewCallback(func(pname *uint16, offset int64, finfo *DokanFileInfo) uintptr {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("SetEndOfFile: not opened?")
		return STATUS_INVALID_PARAMETER
	}

	if trunc, ok := f.file.(interface{ Truncate(int64) error }); ok {
		f.cachedStat = nil
		return errToStatus(trunc.Truncate(offset))
	} else if fsys, ok := f.mi.fsys.(TruncateFS); ok {
		f.cachedStat = nil
		return errToStatus(fsys.Truncate(f.name, offset))
	}
	return STATUS_NOT_SUPPORTED
})
