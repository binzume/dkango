package dkango

import (
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/binzume/dkango/dokan"
)

type openedFile struct {
	mi         *disk
	name       string
	openFlag   int
	cachedStat fs.FileInfo
	file       io.Closer
	pos        int64
}

func (f *openedFile) FindFiles(fillFindDataCallBack func(fi *dokan.WIN32_FIND_DATAW) (bool, error), finfo *dokan.FileInfo) dokan.NTStatus {
	proc := func(files []fs.DirEntry) bool {
		for _, file := range files {
			fi := dokan.WIN32_FIND_DATAW{}
			name, err := dokan.UTF16FromString(file.Name())
			if err != nil {
				log.Println("ERROR: FindFiles", err)
				continue
			}

			copy(fi.FileName[:], name)
			if file.IsDir() {
				fi.FileAttributes = dokan.FILE_ATTRIBUTE_DIRECTORY
			} else {
				fi.FileAttributes = dokan.FILE_ATTRIBUTE_NORMAL
			}
			info, err := file.Info()
			if err == nil {
				fi.FileSizeLow = uint32(info.Size())
				fi.FileSizeHigh = uint32(info.Size() >> 32)
				fi.LastWriteTime = dokan.UnixNanoToFileTime(info.ModTime().UnixNano())
				fi.LastAccessTime = fi.LastWriteTime
				fi.CreationTime = fi.LastWriteTime
			}
			bufferFull, err := fillFindDataCallBack(&fi)
			if err != nil || bufferFull {
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
			if files, err := r.ReadDir(256); len(files) == 0 || !proc(files) || err != nil {
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

func (f *openedFile) GetFileInformation(fi *dokan.ByHandleFileInfo, finfo *dokan.FileInfo) dokan.NTStatus {

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
	fi.LastWriteTime = dokan.UnixNanoToFileTime(f.cachedStat.ModTime().UnixNano())
	fi.LastAccessTime = fi.LastWriteTime
	fi.CreationTime = fi.LastWriteTime
	fi.VolumeSerialNumber = int32(f.mi.opt.VolumeInfo.SerialNumber)

	return dokan.STATUS_SUCCESS
}

func (f *openedFile) ReadFile(buf []byte, read *int32, offset int64, finfo *dokan.FileInfo) dokan.NTStatus {
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
			n, err := r.ReadAt(buf, offset)
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
		n, err := io.ReadFull(r, buf)
		// n, err := r.Read(buf)
		f.pos = offset + int64(n)
		*read = int32(n)
		if n > 0 {
			return dokan.STATUS_SUCCESS // ignore EOF error
		}
		return dokan.ErrorToNTStatus(err)
	}
	return dokan.STATUS_NOT_SUPPORTED
}

func (f *openedFile) WriteFile(buf []byte, written *int32, offset int64, finfo *dokan.FileInfo) dokan.NTStatus {
	if f.openFlag == os.O_RDONLY || f.file == nil {
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
			n, err := w.WriteAt(buf, offset)
			*written = int32(n)
			return dokan.ErrorToNTStatus(err)
		} else {
			return dokan.STATUS_NOT_SUPPORTED
		}
	}
	if r, ok := f.file.(io.Writer); ok {
		n, err := r.Write(buf)
		f.pos = offset + int64(n)
		*written = int32(n)
		return dokan.ErrorToNTStatus(err)
	}
	return dokan.STATUS_NOT_SUPPORTED
}

func (f *openedFile) SetEndOfFile(offset int64, finfo *dokan.FileInfo) dokan.NTStatus {
	if trunc, ok := f.file.(interface{ Truncate(int64) error }); ok {
		f.cachedStat = nil
		return dokan.ErrorToNTStatus(trunc.Truncate(offset))
	} else if fsys, ok := f.mi.fsys.(TruncateFS); ok {
		f.cachedStat = nil
		return dokan.ErrorToNTStatus(fsys.Truncate(f.name, offset))
	}
	return dokan.STATUS_NOT_SUPPORTED
}

func (f *openedFile) MoveFile(newname string, replaceIfExisting bool, finfo *dokan.FileInfo) dokan.NTStatus {
	fsys, ok := f.mi.fsys.(RenameFS)
	if !ok {
		log.Println("WARN: MoveFile: not support Rename()")
		return dokan.STATUS_NOT_SUPPORTED
	}

	newname = strings.TrimPrefix(filepath.ToSlash(newname), "/")
	if newname == "" {
		newname = "."
	}
	f.cachedStat = nil
	return dokan.ErrorToNTStatus(fsys.Rename(f.name, newname))
}

func (f *openedFile) DeleteFile(finfo *dokan.FileInfo) dokan.NTStatus {
	// will be deleted in Cleanup()
	return dokan.STATUS_SUCCESS
}

func (f *openedFile) DeleteDirectory(finfo *dokan.FileInfo) dokan.NTStatus {
	// will be deleted in Cleanup()
	return dokan.STATUS_SUCCESS
}

func (f *openedFile) Cleanup(finfo *dokan.FileInfo) dokan.NTStatus {
	if !finfo.IsDeleteOnClose() {
		return dokan.STATUS_SUCCESS
	}
	if fsys, ok := f.mi.fsys.(RemoveFS); ok {
		return dokan.ErrorToNTStatus(fsys.Remove(f.name))
	}
	return dokan.STATUS_NOT_SUPPORTED
}

func (f *openedFile) CloseFile(*dokan.FileInfo) {
	f.cachedStat = nil
	if f.file != nil {
		f.file.Close()
	}
}
