package dkango

import (
	"io"
	"io/fs"
	"os"
	"path"
	"testing"
	"time"
)

const srcDir = "."
const mountPoint = "X:"

func TestMain(m *testing.M) {
	Init()
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

	stat, err := os.Stat(mountPoint + "\\README.md")
	t.Log("Name: ", stat.Name())
	t.Log("Size: ", stat.Size())
	t.Log("ModTime: ", stat.ModTime())
	t.Log("IsDir: ", stat.IsDir())
	t.Log("Mode: ", stat.Mode())

	b, err := os.ReadFile(mountPoint + "\\README.md")
	if err != nil {
		t.Fatal(err)
	}
	t.Log("Content: ", string(b), err)

	<-time.After(1 * time.Second)

	if mount.OpenedFileCount() != 0 {
		t.Error("Opened files: ", mount.OpenedFileCount())
	}
}

type testWritableFs struct {
	fs.FS
	path string
}

func (fsys *testWritableFs) OpenWriter(name string) (io.WriteCloser, error) {
	return os.OpenFile(path.Join(fsys.path, name), os.O_RDWR|os.O_CREATE, fs.ModePerm)
}

func (fsys *testWritableFs) Truncate(name string, size int64) error {
	return os.Truncate(name, size)
}

func (fsys *testWritableFs) Remove(name string) error {
	return os.Remove(path.Join(fsys.path, name))
}

func TestWritableFS(t *testing.T) {

	mount, err := MountFS(mountPoint, &testWritableFs{FS: os.DirFS(srcDir), path: srcDir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer mount.Close()

	fname := mountPoint + "\\output.txt"

	// TODO
	_ = os.Remove(srcDir + "/output.txt")

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
}
