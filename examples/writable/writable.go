package main

import (
	"io"
	"io/fs"
	"os"
	"path"

	"github.com/binzume/dkango"
)

type WritableDirFS struct {
	fs.FS
	path string
}

func (fsys *WritableDirFS) OpenWriter(name string) (io.WriteCloser, error) {
	return os.OpenFile(path.Join(fsys.path, name), os.O_RDWR|os.O_CREATE, fs.ModePerm)
}

func (fsys *WritableDirFS) Truncate(name string, size int64) error {
	return os.Truncate(name, size)
}

func (fsys *WritableDirFS) Remove(name string) error {
	return os.Remove(path.Join(fsys.path, name))
}

func main() {
	dkango.Init()
	defer dkango.Shutdown()

	srcDir := "."
	mountPoint := "X:"

	if len(os.Args) > 1 {
		srcDir = os.Args[1]
	}
	if len(os.Args) > 2 {
		mountPoint = os.Args[2]
	}

	mount, _ := dkango.MountFS(mountPoint, &WritableDirFS{FS: os.DirFS(srcDir), path: srcDir}, nil)
	defer mount.Close()

	// Block forever
	select {}
}
