package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mattn/go-isatty"
	"golang.org/x/sync/errgroup"
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

func diskUsage(path string) (int64, error) {
	var usage int64
	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			usage += info.Size()
		}
		return nil
	}
	if err := filepath.Walk(path, walkFn); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return 0, err
	}
	return usage, nil
}

// monitorDiskUsage starts a background goroutine that periodically prints the disk usage of the
// given directory or file.  Returns a done callback that stops the periodic printing.  If there is
// an error during monitoring, that error is returned from the returned done callback and, when the
// done callback is called, written to the provided error pointer if the pointer is non-nil and the
// pointed-to value is nil.
func monitorDiskUsage(prefix, path string, errp *error) func() error {
	if !isatty.IsTerminal(os.Stdout.Fd()) {
		return func() error { return nil }
	}
	gr := &errgroup.Group{}
	quit := make(chan struct{})
	gr.Go(func() error {
		// previous holds how many bytes the previous line contained
		// so that we can clear it in its entirety.
		var previous int
		for {
			usage, err := diskUsage(path)
			if err != nil {
				return err
			}
			fmt.Printf("\r%s", strings.Repeat(" ", previous))
			previous, _ = fmt.Printf("\r%s: %s", prefix, humanizeBytes(usage))
			select {
			case <-quit:
				fmt.Printf("\r")
				return nil
			case <-time.After(250 * time.Millisecond):
				break
			}
		}
	})
	return func() error {
		close(quit)
		err := gr.Wait()
		if errp != nil && *errp == nil {
			*errp = err
		}
		return err
	}
}
