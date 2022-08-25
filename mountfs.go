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

type WritableFS interface {
	fs.FS
	OpenWriter(name string, flag int) (io.WriteCloser, error)
	Truncate(name string, size int64) error
}

type RemoveFS interface {
	fs.FS
	Remove(name string) error
}

type MkdirFS interface {
	fs.FS
	Mkdir(name string, ignored fs.FileMode) error
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
	mi   *MountInfo
	name string
	file io.Closer
	stat fs.FileInfo
}

func (f *openedFile) Close() {
	f.mi.removeFile(f)
	f.stat = nil
	if f.file != nil {
		f.file.Close()
	}
}

func MountFS(mountPoint string, fsys fs.FS, opt *MountOptions) (*MountInfo, error) {
	if opt == nil {
		opt = &MountOptions{
			VolumeName:     "fuse volume",
			FileSystemName: "Dokan",
			TotalBytes:     1024 * 1024 * 1024,
			AvailableBytes: 1024 * 1024 * 1024,
		}
	}
	ctx := &MountInfo{fsys: fsys, opt: opt, openedFiles: map[*openedFile]struct{}{}}
	ctx.mounted.Add(1)
	path := unsafe.Pointer(syscall.StringToUTF16Ptr(mountPoint))
	options := &DokanOptions{
		Version:       205,
		GlobalContext: uint64(uintptr(unsafe.Pointer(ctx))),
		// SingleThread:  1,
		MountPoint: uintptr(path),
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
		FindFiles:          findFiles,
		//FindFilesWithPattern: debugCallback,
		//SetFileAttributes:    debugCallback,
		//SetFileTime:          debugCallback,
		DeleteFile:      deleteFile,
		DeleteDirectory: deleteDir,
		//MoveFile:             debugCallback,
		//SetEndOfFile:         debugCallback,
		//SetAllocationSize:    debugCallback,
		//LockFile:             debugCallback,
		//UnlockFile:           debugCallback,
		//GetFileSecurity:      debugCallback,
		//SetFileSecurity:      debugCallback,
		GetDiskFreeSpace:     getDiskFreeSpace,
		GetVolumeInformation: getVolumeInformation,
		Unmounted:            unmounted,
		// FindStreams:          debugCallback,
		Mounted: mounted,
	}

	handle, err := CreateFileSystem(options, operations)
	if err != nil {
		return nil, err
	}
	ctx.handle = handle
	ctx.options = options
	ctx.operations = operations
	ctx.mounted.Wait()
	return ctx, nil
}

func (c *MountInfo) Close() error {
	return CloseHandle(c.handle)
}

func getMountInfo(finfo *DokanFileInfo) *MountInfo {
	return (*MountInfo)(unsafe.Pointer(uintptr(finfo.DokanOptions.GlobalContext)))
}

func getOpenedFile(finfo *DokanFileInfo) *openedFile {
	return (*openedFile)(unsafe.Pointer(uintptr(finfo.Context)))
}

var debugCallback = syscall.NewCallback(func(param uintptr) uintptr {
	log.Println("debugCallback", param)
	return STATUS_ACCESS_DENINED
})

var getVolumeInformation = syscall.NewCallback(func(pName *uint16, nameSize int32, serial *uint32, maxCLen *uint32, flags *uint32, pSysName *uint16, sysNameSize int32, finfo *DokanFileInfo) uintptr {
	dk := getMountInfo(finfo)
	if dk == nil {
		return STATUS_ACCESS_DENINED
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

	stat, err := fs.Stat(mi.fsys, name)
	if !(create && errors.Is(err, fs.ErrNotExist)) && err != nil {
		return STATUS_OBJECT_NAME_NOT_FOUND
	}
	if disposition == FILE_CREATE && options&FILE_DIRECTORY_FILE != 0 {
		if fsys, ok := mi.fsys.(MkdirFS); ok {
			fsys.Mkdir(name, fs.ModePerm)
		} else {
			return STATUS_NOT_SUPPORTED
		}
	}
	if access&FILE_WRITE_DATA != 0 {
		_, ok := mi.fsys.(interface {
			OpenWriter(string, int) (io.WriteCloser, error)
		})
		if !ok {
			// Read only FS
			return STATUS_ACCESS_DENINED
		}
	}

	if truncate {
		// TODO: Support Open(name) and f.Truncate(size)
		if fsys, ok := mi.fsys.(WritableFS); ok {
			fsys.Truncate(name, 0)
		}
	}

	f := &openedFile{name: name, mi: mi, stat: stat}
	mi.addFile(f) // avoid GC
	finfo.Context = uint64(uintptr(unsafe.Pointer(f)))

	return STATUS_SUCCESS
})

var findFiles = syscall.NewCallback(func(pname *uint16, fillFindData uintptr, finfo *DokanFileInfo) uintptr {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("FindFIles not opened?")
		return STATUS_ACCESS_DENINED
	}
	if f.name == "" {
		f.name = "."
	}

	files, err := fs.ReadDir(f.mi.fsys, f.name)
	if err != nil {
		log.Println("ReadDIR", f.name, err)
		return STATUS_ACCESS_DENINED
	}
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
		syscall.SyscallN(fillFindData, uintptr(unsafe.Pointer(&fi)), uintptr(unsafe.Pointer(finfo)))
	}

	return STATUS_SUCCESS
})

var getFileInformation = syscall.NewCallback(func(pname *uint16, fi *ByHandleFileInfo, finfo *DokanFileInfo) uintptr {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("GetFileInfo: not opened?")
		return STATUS_ACCESS_DENINED
	}

	if f.stat == nil {
		stat, err := fs.Stat(f.mi.fsys, f.name)
		if err == nil {
			f.stat = stat
		}
	}
	if f.stat == nil {
		return STATUS_ACCESS_DENINED
	}
	if f.stat.IsDir() {
		fi.FileAttributes = FILE_ATTRIBUTE_DIRECTORY
	} else {
		fi.FileAttributes = FILE_ATTRIBUTE_NORMAL
	}
	fi.FileSizeLow = uint32(f.stat.Size())
	fi.FileSizeHigh = uint32(f.stat.Size() >> 32)
	t := (f.stat.ModTime().UnixNano())/100 + UnixTimeOffset
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
		return STATUS_ACCESS_DENINED
	}

	if f.file == nil {
		r, err := f.mi.fsys.Open(f.name)
		if err != nil {
			return STATUS_ACCESS_DENINED
		}
		f.file = r
	}

	if r, ok := f.file.(io.ReadSeeker); ok {
		r.Seek(offset, 0)
		n, err := r.Read(unsafe.Slice(buf, sz))
		*read = int32(n)
		if errors.Is(err, io.EOF) {
			return STATUS_END_OF_FILE
		}
		return STATUS_SUCCESS
	}
	return STATUS_ACCESS_DENINED
})

var writeFile = syscall.NewCallback(func(pname *uint16, buf *byte, sz int32, written *int32, offset int64, finfo *DokanFileInfo) uintptr {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("WriteFile: not opened?")
		return STATUS_ACCESS_DENINED
	}

	fsys, ok := f.mi.fsys.(interface {
		OpenWriter(string, int) (io.WriteCloser, error)
	})
	if !ok {
		log.Println("WriteFile: not support OpenWriter?")
		return STATUS_NOT_SUPPORTED
	}
	f.stat = nil // invalidate cached stat

	if f.file == nil {
		r, err := fsys.OpenWriter(f.name, syscall.O_RDWR|syscall.O_CREAT)
		if err != nil {
			return STATUS_ACCESS_DENINED
		}
		f.file = r
	}

	if w, ok := f.file.(io.WriteSeeker); ok {
		w.Seek(offset, 0)
		n, err := w.Write(unsafe.Slice(buf, sz))
		*written = int32(n)
		if errors.Is(err, io.EOF) {
			return STATUS_END_OF_FILE
		}
		return STATUS_SUCCESS
	}
	return STATUS_ACCESS_DENINED
})

var cleanup = syscall.NewCallback(func(pname *uint16, finfo *DokanFileInfo) uintptr {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("CloseFile: not opened?")
		return STATUS_ACCESS_DENINED
	}

	if finfo.DeleteOnClose == 0 {
		return STATUS_SUCCESS
	}
	if fsys, ok := f.mi.fsys.(RemoveFS); ok {
		err := fsys.Remove(f.name)
		if err != nil {
			return STATUS_ACCESS_DENINED
		}
	}
	return STATUS_ACCESS_DENINED
})

var closeFile = syscall.NewCallback(func(pname *uint16, finfo *DokanFileInfo) uintptr {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("CloseFile: not opened?")
		return STATUS_ACCESS_DENINED
	}
	f.Close()
	finfo.Context = 0
	return STATUS_SUCCESS
})

var deleteFile = syscall.NewCallback(func(pname *uint16, finfo *DokanFileInfo) uintptr {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("DeleteFile: not opened?")
		return STATUS_ACCESS_DENINED
	}
	return STATUS_SUCCESS
})

var deleteDir = syscall.NewCallback(func(pname *uint16, finfo *DokanFileInfo) uintptr {
	f := getOpenedFile(finfo)
	if f == nil {
		log.Println("DeleteDir: not opened?")
		return STATUS_ACCESS_DENINED
	}
	return STATUS_SUCCESS
})
