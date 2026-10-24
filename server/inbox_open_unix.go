//go:build !windows && !plan9

package server

import (
	"golang.org/x/sys/unix"
	"os"
)

func openInboxFile(root *os.Root, name string) (*os.File, error) {
	return root.OpenFile(name, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
}
