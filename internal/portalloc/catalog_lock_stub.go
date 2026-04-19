//go:build !unix

package portalloc

import (
	"fmt"
	"runtime"
	"time"
)

func acquireCatalogLock(timeout time.Duration) (*catalogLock, error) {
	return nil, fmt.Errorf("catalog locking is not supported on %s", runtime.GOOS)
}

func (l *catalogLock) Close() error {
	return nil
}
