//go:build !unix

package portalloc

import (
	"fmt"
	"runtime"
	"time"
)

func acquireCatalogLock(timeout time.Duration) (*catalogLock, error) {
	return nil, fmt.Errorf(
		"catalog locking is not supported on %s: Windows support is tracked under "+
			"\"Windows support for catalog concurrency\" in plans/phase-roadmap.md (Deep roadmap, not yet scheduled)",
		runtime.GOOS,
	)
}

func (l *catalogLock) Close() error {
	return nil
}
