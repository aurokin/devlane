//go:build !unix

package portalloc

import "fmt"

func listenTCP6(port int) error {
	return listenAndClose("tcp6", fmt.Sprintf("[::]:%d", port))
}

func isIPv6Unsupported(err error) bool {
	return false
}
