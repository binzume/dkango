package dokan

import (
	"os"
	"testing"
)

type nopDisk struct{}

func (*nopDisk) GetVolumeInformation(finfo *FileInfo) (VolumeInformation, NTStatus) {
	return VolumeInformation{}, STATUS_NOT_SUPPORTED
}
func (*nopDisk) GetDiskFreeSpace(availableBytes *uint64, totalBytes *uint64, freeBytes *uint64, finfo *FileInfo) NTStatus {
	return STATUS_NOT_SUPPORTED
}
func (*nopDisk) CreateFile(name string, secCtx uintptr, access, attrs, share, disposition, options uint32, finfo *FileInfo) (FileHandle, NTStatus) {
	return nil, STATUS_NOT_SUPPORTED
}

func TestMountDisk_Dir(t *testing.T) {
	mountPoint := "./testmountpoont"
	os.Mkdir(mountPoint, os.ModePerm)

	n, err := MountPoints()
	if err != nil {
		t.Fatal(err)
	}
	if len(n) != 0 {
		t.Error("mount points != 0: ", n)
	}

	mi, err := MountDisk(mountPoint, &nopDisk{}, 0)
	if err != nil {
		t.Fatal("MountDisk() error", err)
	}

	n, err = MountPoints()
	if err != nil {
		t.Fatal(err)
	}
	if len(n) != 1 {
		t.Error("mount points != 1: ", n)
	}

	err = mi.Close()
	if err != nil {
		t.Fatal("Close() error", err)
	}
}

func TestMountDisk_Drive(t *testing.T) {
	mountPoint := "Y:"
	n, err := MountPoints()
	if err != nil {
		t.Fatal(err)
	}
	if len(n) != 0 {
		t.Error("mount points != 0: ", n)
	}

	mi, err := MountDisk(mountPoint, &nopDisk{}, 0)
	if err != nil {
		t.Fatal("MountDisk() error", err)
	}

	n, err = MountPoints()
	if err != nil {
		t.Fatal(err)
	}
	if len(n) != 1 {
		t.Error("mount points != 1: ", n)
	}

	err = mi.Close()
	if err != nil {
		t.Fatal("Close() error", err)
	}
}
