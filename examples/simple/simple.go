package main

import (
	"os"

	"github.com/binzume/dkango"
)

func main() {
	mount, _ := dkango.MountFS("X:", os.DirFS("."), nil)
	defer mount.Close()

	// Block forever
	select {}
}
