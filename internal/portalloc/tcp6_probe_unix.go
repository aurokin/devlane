//go:build unix

package portalloc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"syscall"
)

func listenTCP6(port int) error {
	listenConfig := net.ListenConfig{
		Control: func(_, _ string, rawConn syscall.RawConn) error {
			var controlErr error
			if err := rawConn.Control(func(fd uintptr) {
				controlErr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IPV6, syscall.IPV6_V6ONLY, 1)
			}); err != nil {
				return err
			}
			return controlErr
		},
	}

	listener, err := listenConfig.Listen(context.Background(), "tcp6", fmt.Sprintf("[::]:%d", port))
	if err != nil {
		return err
	}
	return listener.Close()
}

func isIPv6Unsupported(err error) bool {
	return errors.Is(err, syscall.EAFNOSUPPORT) ||
		errors.Is(err, syscall.EPROTONOSUPPORT) ||
		errors.Is(err, syscall.ENOPROTOOPT)
}
