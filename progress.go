package main

import (
	"fmt"
	"github.com/mattn/go-isatty"
	"os"
	"path/filepath"
	"strings"
	"time"
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

func progressSize(prefix, path string, done chan struct{}) {
	// previous holds how many bytes the previous line contained
	// so that we can clear it in its entirety.
	var previous int
	tty := isatty.IsTerminal(os.Stdout.Fd())
	for {
		if tty {
			var usage int64
			filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
				if err == nil && info.Mode().IsRegular() {
					usage += info.Size()
				}
				return nil
			})
			fmt.Printf("\r%s", strings.Repeat(" ", previous))
			previous, _ = fmt.Printf("\r%s: %s", prefix, humanizeBytes(usage))
		}

		select {
		case <-done:
			fmt.Printf("\r")
			return
		case <-time.After(250 * time.Millisecond):
			break
		}
	}
}
