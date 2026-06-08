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
	err := filepath.Walk(path, func(p string, info fs.FileInfo, err error) error {
		// IMPORTANT: always check err first
		if err != nil {
			return err
		}

		if info == nil {
			return nil
		}

		if info.IsDir() {
			return os.Chmod(p, 0777)
		}

		// remove files
		return os.Remove(p)
	})

	if err != nil {
		return err
	}

	// remove the remaining directories
	return os.RemoveAll(path)
}
