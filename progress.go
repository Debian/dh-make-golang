package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mattn/go-isatty"
)

const (
	_ = 1 << (10 * iota)
	Kibi
	Mebi
	Gibi
	Tebi
)

func humanizeBytes(b int64) string {
	if b > Tebi {
		return fmt.Sprintf("%.2f TiB", float64(b)/float64(Tebi))
	} else if b > Gibi {
		return fmt.Sprintf("%.2f GiB", float64(b)/float64(Gibi))
	} else if b > Mebi {
		return fmt.Sprintf("%.2f MiB", float64(b)/float64(Mebi))
	} else {
		return fmt.Sprintf("%.2f KiB", float64(b)/float64(Kibi))
	}
}

func diskUsage(path string) int64 {
	var usage int64
	walkFn := func(path string, info os.FileInfo, err error) error {
		if err == nil && info.Mode().IsRegular() {
			usage += info.Size()
		}
		return nil
	}
	filepath.Walk(path, walkFn)
	return usage
}

// monitorDiskUsage starts a background goroutine that periodically prints the disk usage of the
// given directory or file.  Returns a done callback that stops the periodic printing.
func monitorDiskUsage(prefix, path string) func() {
	if !isatty.IsTerminal(os.Stdout.Fd()) {
		return func() {}
	}
	done := make(chan struct{})
	go func() {
		// previous holds how many bytes the previous line contained
		// so that we can clear it in its entirety.
		var previous int
		for {
			usage := diskUsage(path)
			fmt.Printf("\r%s", strings.Repeat(" ", previous))
			previous, _ = fmt.Printf("\r%s: %s", prefix, humanizeBytes(usage))
			select {
			case <-done:
				fmt.Printf("\r")
				return
			case <-time.After(250 * time.Millisecond):
				break
			}
		}
	}()
	return func() { close(done) }
}
