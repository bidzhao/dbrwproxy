//go:build !linux && !darwin && !dragonfly && !freebsd && !netbsd && !openbsd && !solaris && !illumos && !windows
// +build !linux,!darwin,!dragonfly,!freebsd,!netbsd,!openbsd,!solaris,!illumos,!windows

package mysql

import "net"

func connCheck(conn net.Conn) error {
	return nil
}
