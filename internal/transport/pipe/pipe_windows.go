//go:build windows

package pipe

import (
	"context"
	"fmt"
	"io"
	"os"
	"syscall"
)

func openPipe(ctx context.Context, pipePath string) (io.ReadWriteCloser, error) {
	type dialResult struct {
		file *os.File
		err  error
	}
	ch := make(chan dialResult, 1)
	go func() {
		path16, err := syscall.UTF16PtrFromString(pipePath)
		if err != nil {
			ch <- dialResult{err: err}
			return
		}
		h, err := syscall.CreateFile(
			path16,
			syscall.GENERIC_READ|syscall.GENERIC_WRITE,
			0,
			nil,
			syscall.OPEN_EXISTING,
			syscall.FILE_FLAG_OVERLAPPED,
			0,
		)
		if err != nil {
			ch <- dialResult{err: err}
			return
		}
		ch <- dialResult{file: os.NewFile(uintptr(h), pipePath)}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-ch:
		if res.err != nil {
			return nil, fmt.Errorf("dial pipe %q: %w", pipePath, res.err)
		}
		return res.file, nil
	}
}
