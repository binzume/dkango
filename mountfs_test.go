package dkango

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"testing"
)

const srcDir = "."
const mountPoint = "X:"

func TestMountFS(t *testing.T) {
	n, err := MountPoints()
	if err != nil {
		t.Fatal(err)
	}
	if len(n) != 0 {
		t.Error("mount points != 0: ", n)
	}

	mount, err := MountFS(mountPoint, os.DirFS(srcDir), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer mount.Close()

	n, err = MountPoints()
	if err != nil {
		t.Fatal(err)
	}
	if len(n) != 1 {
		t.Error("mount points != 1: ", n)
	}

	if err != nil {
		t.Error("ReadDir() error", err)
	}

	fname := mountPoint + "/LICENSE"

	stat, err := os.Stat(fname)
	t.Log("Name: ", stat.Name())
	t.Log("Size: ", stat.Size())
	t.Log("ModTime: ", stat.ModTime())
	t.Log("IsDir: ", stat.IsDir())
	t.Log("Mode: ", stat.Mode())

	r, err := os.Open(fname)
	if err != nil {
		t.Fatal("Open() error", err)
	}

	_, err = r.Write([]byte("Test"))
	if err == nil {
		t.Error("Write() shoudl be failed")
	}

	buf := make([]byte, 32)
	_, err = r.Read(buf)
	if err != nil {
		t.Error("Read() error", err)
	}

	_, err = r.ReadAt(buf, 8)
	if err != nil {
		t.Error("Read() error", err)
	}

	err = r.Close()
	if err != nil {
		t.Error("Close() error", err)
	}

	r, err = os.Open(mountPoint + "/notfound")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Error("Open() for not exitst file shoudl be failed.", err)
	}

	r, err = os.OpenFile(mountPoint+"/notfound", os.O_CREATE|os.O_WRONLY, 0)
	if !errors.Is(err, fs.ErrPermission) {
		t.Error("OpenFile() wtih O_CREATE shoudl be failed.", err)
	}

	b, err := os.ReadFile(fname)
	if err != nil {
		t.Error("ReadFile() error", err)
	}
	t.Log("Content: ", string(b))

	if mount.OpenedFileCount() != 0 {
		// Not an issue because other processeses maybe open files
		t.Log("Opened files: ", mount.OpenedFileCount())
	}
}

type testWritableFs struct {
	fs.FS
	path string
}

func (fsys *testWritableFs) OpenWriter(name string, flag int) (io.WriteCloser, error) {
	return os.OpenFile(path.Join(fsys.path, name), flag, fs.ModePerm)
}

func (fsys *testWritableFs) Remove(name string) error {
	return os.Remove(path.Join(fsys.path, name))
}

func (fsys *testWritableFs) Mkdir(name string, mode fs.FileMode) error {
	return os.Mkdir(path.Join(fsys.path, name), mode)
}

func (fsys *testWritableFs) Rename(name, newName string) error {
	return os.Rename(path.Join(fsys.path, name), path.Join(fsys.path, newName))
}

func TestWritableFS(t *testing.T) {
	// OptionFlags = DOKAN_OPTION_ALT_STREAM | DOKAN_OPTION_DEBUG | DOKAN_OPTION_STDERR
	mount, err := MountFS(mountPoint, &testWritableFs{FS: os.DirFS(srcDir), path: srcDir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer mount.Close()

	fname := mountPoint + "\\output.txt"

	_ = os.Remove(fname)
	_ = os.Remove(fname + ".renamed")

	files, err := os.ReadDir(mountPoint)
	if err != nil {
		t.Fatal("ReadDir() error", err)
	}
	t.Log("ReadDir() files: ", len(files))

	f, err := os.Create(fname)
	if err != nil {
		t.Fatal("Create() error", err)
	}

	_, err = f.Write([]byte("hello, FUSE!\n"))
	if err != nil {
		t.Fatal("Write() error", err)
	}
	_, err = f.Write([]byte("1234567890"))
	if err != nil {
		t.Fatal("Write() error", err)
	}

	_, err = f.WriteAt([]byte("Hello"), 0)
	if err != nil {
		t.Fatal("WriteAt() error", err)
	}

	err = f.Close()
	if err != nil {
		t.Fatal("Close() error", err)
	}

	err = os.Truncate(fname, 1)
	if err != nil {
		t.Error("Truncate() error", err)
	}

	err = os.Rename(fname, fname+".renamed")
	if err != nil {
		t.Fatal("Remove() error", err)
	}

	err = os.Remove(fname + ".renamed")
	if err != nil {
		t.Fatal("Remove() error", err)
	}

	dname := mountPoint + "\\dir"

	err = os.Mkdir(dname, fs.ModePerm)
	if err != nil {
		t.Fatal("Mkdir() error", err)
	}

	err = os.Remove(dname)
	if err != nil {
		t.Fatal("Remove() dir error", err)
	}

	f, err = os.OpenFile(fname, os.O_WRONLY, os.ModePerm)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("OpenFile() should fail with ErrNotExist", err)
	}

	// Create empty file
	f, err = os.Create(fname)
	if err != nil {
		t.Fatal("Create() error", err)
	}
	err = f.Close()
	if err != nil {
		t.Fatal("Close() error", err)
	}

	// Append1
	f, err = os.OpenFile(fname, os.O_WRONLY|os.O_APPEND, os.ModePerm)
	if err != nil {
		t.Fatal("OpenFile() error", err)
	}
	_, err = f.Write([]byte("01234"))
	if err != nil {
		t.Fatal("Write() error", err)
	}
	err = f.Close()
	if err != nil {
		t.Fatal("Close() error", err)
	}

	// Append2
	f, err = os.OpenFile(fname, os.O_WRONLY|os.O_APPEND, os.ModePerm)
	if err != nil {
		t.Fatal("OpenFile() error", err)
	}
	_, err = f.Write([]byte("56789"))
	if err != nil {
		t.Fatal("Write() error", err)
	}
	err = f.Close()
	if err != nil {
		t.Fatal("Close() error", err)
	}

	stat, err := os.Stat(fname)
	if stat.Size() != 10 {
		t.Fatal("Size() should returns 10", stat.Size())
	}

	// Err if exists
	f, err = os.OpenFile(fname, os.O_WRONLY|os.O_CREATE|os.O_EXCL, os.ModePerm)
	if !errors.Is(err, fs.ErrExist) {
		t.Fatal("OpenFile() should fail with ErrExist", err)
	}

	err = os.Remove(fname)
	if err != nil {
		t.Fatal("Remove() error", err)
	}
}
