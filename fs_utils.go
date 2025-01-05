package main

import (
	"io/fs"
	"os"
	"path/filepath"
)

// isFile checks if a path exists and is a file (not a directory).
func isFile(path string) bool {
	if info, err := os.Stat(path); err == nil {
		return !info.IsDir()
	}
	return false
}

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
