//go:build !windows

package pipe

import (
	"context"
	"fmt"
	"io"
)

func openPipe(ctx context.Context, pipePath string) (io.ReadWriteCloser, error) {
	return nil, fmt.Errorf("named pipes require windows: %s", pipePath)
}
