package main

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
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
	log.Printf("%s: monitoring disk usage in %s", prefix, path)
	before, err := diskUsage(path)
	if err != nil {
		return func() error {
			if errp != nil && *errp == nil {
				*errp = err
			}
			return err
		}
	}
	firstRun := true
	render := func() (string, error) {
		after := before
		if firstRun {
			firstRun = false
		} else {
			var err error
			if after, err = diskUsage(path); err != nil {
				return "", err
			}
		}
		hDiff := humanizeBytes(after)
		total := ""
		if before != 0 {
			total = fmt.Sprintf(" (%s total)", hDiff)
			hDiff = humanizeBytes(after - before)
		}
		return fmt.Sprintf("%s: %s added%s", prefix, hDiff, total), nil
	}
	return monitorProgress(render, errp)
}

func monitorProgress(render func() (string, error), errp *error) func() error {
	gr := &errgroup.Group{}
	quit := make(chan struct{})
	period := 3 * time.Second
	output := func(out string) { log.Print(out) }
	clear := func() {}
	if isatty.IsTerminal(os.Stdout.Fd()) {
		period = 250 * time.Millisecond
		// TODO: This approach doesn't work well in two cases:
		//
		//   * out is longer than a line.  The clear function only clears the current line, which will
		//     be the last line of the wrapped message.  Example:
		//
		//         foo 123 of 456 in /really/long/path/name/that/wra
		//         foo 124 of 456 in /really/long/path/name/that/wra
		//         foo 125 of 456 in /really/long/path/name/that/wra
		//         ps/to/the/next/line
		//
		//   * A message is logged.  The log message will print immediately after out on the same line,
		//     and the next refresh of the displayed progress will be on a different line.  Example:
		//
		//         foo 123 of 4562026/03/09 15:03:53 log message here
		//         foo 124 of 456
		//
		// Perhaps a TUI library (or ncurses directly) can be used to create two areas: a region at the
		// bottom of the screen for progress and the rest of the screen for displaying log messages.
		output = func(out string) {
			clear()
			fmt.Print(out)
		}
		clear = func() {
			const csi = "\033["   // Control Sequence Introducer
			const el0 = csi + "K" // Erase in Line 0 (clear from cursor to end of line)
			fmt.Print("\r" + el0)
		}
	}
	out, err := render()
	if err != nil {
		return func() error {
			if errp != nil && *errp == nil {
				*errp = err
			}
			return err
		}
	}
	gr.Go(func() error {
		tickCh := time.Tick(period)
		for {
			output(out)
			select {
			case <-quit:
			case <-tickCh:
			}
			out, err = render()
			if err != nil {
				return err
			}
			select {
			case <-quit:
				return nil
			default:
			}
		}
	})
	return func() error {
		close(quit)
		if err := gr.Wait(); err != nil {
			if errp != nil && *errp == nil {
				*errp = err
			}
			return err
		}
		clear()
		log.Print(out)
		return nil
	}
}
