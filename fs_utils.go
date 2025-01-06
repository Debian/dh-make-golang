package main

import (
	"io/fs"
	"os"
	"path/filepath"
)

// forceRemoveAll is a more robust alternative to [os.RemoveAll] that tries
// harder to remove all the files and directories.
func forceRemoveAll(path string) error {
	// first pass to make sure all the directories are writable
	err := filepath.Walk(path, func(path string, info fs.FileInfo, err error) error {
		if info.IsDir() {
			return os.Chmod(path, 0777)
		} else {
			// remove files by the way
			return os.Remove(path)
		}
	})
	if err != nil {
		return err
	}
	// remove the remaining directories
	return os.RemoveAll(path)
}
