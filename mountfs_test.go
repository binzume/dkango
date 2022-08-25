package dkango

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"testing"
)

const srcDir = "."
const mountPoint = "X:"

func TestMain(m *testing.M) {
	err := Init()
	if err != nil {
		fmt.Println("Failed to initialize:", err)
		os.Exit(1)
	}
	defer Shutdown()
	os.Exit(m.Run())
}

func TestMountFS(t *testing.T) {
	n, err := MountPoints()
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
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
	if n != 1 {
		t.Error("mount points != 1: ", n)
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

	buf := make([]byte, 100)
	_, err = r.Read(buf)
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

func (fsys *testWritableFs) Truncate(name string, size int64) error {
	return os.Truncate(name, size)
}

func (fsys *testWritableFs) Remove(name string) error {
	return os.Remove(path.Join(fsys.path, name))
}

func (fsys *testWritableFs) Mkdir(name string, mode fs.FileMode) error {
	return os.Mkdir(path.Join(fsys.path, name), mode)
}

func TestWritableFS(t *testing.T) {

	mount, err := MountFS(mountPoint, &testWritableFs{FS: os.DirFS(srcDir), path: srcDir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer mount.Close()

	fname := mountPoint + "\\output.txt"

	_ = os.Remove(fname)

	f, err := os.Create(fname)
	if err != nil {
		t.Fatal("Create() error", err)
	}

	_, err = f.Write([]byte("Hello, FUSE!\n"))
	if err != nil {
		t.Fatal("Write() error", err)
	}
	_, err = f.Write([]byte("1234567890"))
	if err != nil {
		t.Fatal("Write() error", err)
	}

	err = f.Close()
	if err != nil {
		t.Fatal("Close() error", err)
	}

	err = os.Remove(fname)
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
}
