# Dokan bindings for Go

Dokan: https://ja.wikipedia.org/wiki/Dokan

- Depends only on the Dokan library (No cgo)
- fs.FS interface support

## Usage

### Install Dokan library

```sh
winget install dokan-dev.Dokany
```

### examples/simple/simple.go

```go
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
```

### How to create a writable FS?

See examples/writable/writable.go

```
go run ./examples/writable testdir R:
```

```go
type OpenWriterFS interface {
	fs.FS
	OpenWriter(name string, flag int) (io.WriteCloser, error)
}
```

Other interfaces such as RemoveFS, MkdirFS, RenameFS... are also available.

## TODO

- Notify event
- Performance

## License

MIT
