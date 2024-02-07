package dkango

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/binzume/dkango/dokan"
)

type disk struct {
	opt  *MountOptions
	fsys fs.FS
}

func (d *disk) GetVolumeInformation(finfo *dokan.FileInfo) (dokan.VolumeInformation, dokan.NTStatus) {
	return d.opt.VolumeInfo, dokan.STATUS_SUCCESS
}

func (d *disk) GetDiskFreeSpace(availableBytes *uint64, totalBytes *uint64, freeBytes *uint64, finfo *dokan.FileInfo) dokan.NTStatus {
	if (d.opt.DiskSpaceFunc) != nil {
		space := d.opt.DiskSpaceFunc()
		*availableBytes = space.FreeBytesAvailable
		*totalBytes = space.TotalNumberOfBytes
		*freeBytes = space.TotalNumberOfFreeBytes
		return dokan.STATUS_SUCCESS
	}
	return dokan.STATUS_NOT_SUPPORTED
}

func (mi *disk) CreateFile(name string, secCtx uintptr, access, attrs, share, disposition, options uint32, finfo *dokan.FileInfo) (dokan.FileHandle, dokan.NTStatus) {
	name = strings.TrimPrefix(filepath.ToSlash(name), "/")
	if name == "" {
		name = "."
	}

	create := disposition == dokan.FILE_CREATE || disposition == dokan.FILE_OPEN_IF || disposition == dokan.FILE_OVERWRITE_IF || disposition == dokan.FILE_SUPERSEDE
	truncate := disposition == dokan.FILE_SUPERSEDE || disposition == dokan.FILE_OVERWRITE || disposition == dokan.FILE_OVERWRITE_IF
	errIfExist := disposition == dokan.FILE_CREATE
	openFlag := 0
	if access&dokan.FILE_WRITE_DATA != 0 && access&dokan.FILE_READ_DATA != 0 {
		openFlag = os.O_RDWR
	} else if access&dokan.FILE_READ_DATA != 0 {
		openFlag = os.O_RDONLY
	} else if access&dokan.FILE_WRITE_DATA != 0 {
		openFlag = os.O_WRONLY
	} else if access&dokan.FILE_APPEND_DATA != 0 {
		openFlag = os.O_WRONLY | os.O_APPEND
	}

	if openFlag == os.O_RDWR || openFlag == os.O_WRONLY {
		if create {
			openFlag |= os.O_CREATE
		}
		if truncate {
			openFlag |= os.O_TRUNC
		}
		if errIfExist {
			openFlag |= os.O_EXCL
		}
	}

	stat, err := fs.Stat(mi.fsys, name)
	if err != nil && !(create && errors.Is(err, fs.ErrNotExist)) {
		return nil, dokan.ErrorToNTStatus(err) // Unexpected error
	}
	if err == nil && disposition == dokan.FILE_CREATE {
		return nil, dokan.STATUS_OBJECT_NAME_COLLISION
	}
	if err == nil && stat.IsDir() && options&dokan.FILE_NON_DIRECTORY_FILE != 0 {
		return nil, dokan.STATUS_FILE_IS_A_DIRECTORY
	}
	if err == nil && !stat.IsDir() && options&dokan.FILE_DIRECTORY_FILE != 0 {
		return nil, dokan.STATUS_NOT_A_DIRECTORY
	}

	f := &openedFile{name: name, mi: mi, cachedStat: stat, openFlag: openFlag}

	// Mkdir
	if create && options&dokan.FILE_DIRECTORY_FILE != 0 {
		if fsys, ok := mi.fsys.(MkdirFS); ok {
			err = fsys.Mkdir(name, fs.ModePerm)
			if err != nil {
				return nil, dokan.ErrorToNTStatus(err)
			}
		} else {
			return nil, dokan.STATUS_NOT_SUPPORTED
		}
	}

	// NOTE: Reader is not opened here because sometimes it may only need GetFileInformantion()
	if openFlag != os.O_RDONLY && options&dokan.FILE_DIRECTORY_FILE == 0 {
		fsys, ok := f.mi.fsys.(OpenWriterFS)
		if !ok {
			// Readonly FS. TODO: Consider to return STATUS_NOT_SUPPORTED?
			return nil, dokan.STATUS_ACCESS_DENIED
		}
		if truncate {
			f.cachedStat = nil // file size will be cahnged
		}
		w, err := fsys.OpenWriter(name, openFlag)
		if err != nil {
			return nil, dokan.ErrorToNTStatus(err)
		}
		f.file = w
	}

	return f, dokan.STATUS_SUCCESS
}
