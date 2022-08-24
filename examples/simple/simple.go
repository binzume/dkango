package main

import (
	"os"

	"github.com/binzume/dkango"
)

func main() {
	dkango.Init()
	defer dkango.Shutdown()

	mount, _ := dkango.MountFS("X:", os.DirFS("."), nil)
	defer mount.Close()

	// Block forever
	select {}
}
