package main

import (
	"fmt"
	"path/filepath"
)

func main() {
	fmt.Println(filepath.Join(".", "/foo/bar"))
	fmt.Println(filepath.Join("/tmp/dir", "../etc/passwd"))
}
