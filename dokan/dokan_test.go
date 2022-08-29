package dokan

import (
	"testing"
)

func TestVersion(t *testing.T) {
	n, err := Version()
	if err != nil {
		t.Fatal("Version() error", err)
	}
	if n < DOKAN_MINIMUM_COMPATIBLE_VERSION {
		t.Error("Dokan version error ", n)
	}
	t.Log("Dokan version:", n)

	n, err = DriverVersion()
	if err != nil {
		t.Fatal("DriverVersion() error", err)
	}
	if n == 0 {
		t.Error("Dokan driver version error", n)
	}
	t.Log("Driver version:", n)
}

func TestInit(t *testing.T) {
	err := Init()
	if err != nil {
		t.Fatal("Init() error", err)
	}

	err = Shutdown()
	if err != nil {
		t.Fatal("Shutdown() error", err)
	}
}

func TestMountPoints(t *testing.T) {
	mp, err := MountPoints()
	if err != nil {
		t.Fatal("MountPoints() error", err)
	}
	t.Log("MountPoints: ", mp)
}
