package web

import (
	"embed"
	"io/fs"
)

//go:embed index.html css js vendor
var FS embed.FS

func Sub() fs.FS {
	sub, err := fs.Sub(FS, ".")
	if err != nil {
		panic(err)
	}
	return sub
}
