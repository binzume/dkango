package dkango

import (
	"os"
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

	<-time.After(15 * time.Second)

	if mount.OpenedFileCount() != 0 {
		t.Error("Opened files: ", mount.OpenedFileCount())
	}
}
