package main

import (
	"os"

	"github.com/binzume/dkango"
)

func main() {
	mount, err := dkango.MountFS("X:", os.DirFS("."), nil)
	if err != nil {
		panic(err)
	}
	defer mount.Close()

	// Block forever
	select {}
}
