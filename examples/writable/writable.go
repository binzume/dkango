package main

import (
	"io"
	"io/fs"
	"os"
	"path"

	"github.com/binzume/dkango"
)

type writableDirFS struct {
	fs.StatFS
	path string
}

func NewWritableDirFS(path string) *writableDirFS {
	return &writableDirFS{StatFS: os.DirFS(path).(fs.StatFS), path: path}
}

func (fsys *writableDirFS) OpenWriter(name string, flag int) (io.WriteCloser, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	return os.OpenFile(path.Join(fsys.path, name), flag, fs.ModePerm)
}

func (fsys *writableDirFS) Truncate(name string, size int64) error {
	if !fs.ValidPath(name) {
		return &fs.PathError{Op: "truncate", Path: name, Err: fs.ErrInvalid}
	}
	return os.Truncate(name, size)
}

func (fsys *writableDirFS) Remove(name string) error {
	if !fs.ValidPath(name) {
		return &fs.PathError{Op: "remove", Path: name, Err: fs.ErrInvalid}
	}
	return os.Remove(path.Join(fsys.path, name))
}

func (fsys *writableDirFS) Mkdir(name string, mode fs.FileMode) error {
	if !fs.ValidPath(name) {
		return &fs.PathError{Op: "mkdir", Path: name, Err: fs.ErrInvalid}
	}
	return os.Mkdir(path.Join(fsys.path, name), mode)
}

func (fsys *writableDirFS) Rename(name, newName string) error {
	if !fs.ValidPath(name) || !fs.ValidPath(newName) {
		return &fs.PathError{Op: "rename", Path: name, Err: fs.ErrInvalid}
	}
	return os.Rename(path.Join(fsys.path, name), path.Join(fsys.path, newName))
}

func main() {
	srcDir := "."
	mountPoint := "X:"

	if len(os.Args) > 1 {
		srcDir = os.Args[1]
	}
	if len(os.Args) > 2 {
		mountPoint = os.Args[2]
	}

	mount, err := dkango.MountFS(mountPoint, NewWritableDirFS(srcDir), nil)
	if err != nil {
		panic(err)
	}
	defer mount.Close()

	// Block forever
	select {}
}
