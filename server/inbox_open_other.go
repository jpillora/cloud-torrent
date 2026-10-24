//go:build windows || plan9

package server

import "os"

func openInboxFile(root *os.Root, name string) (*os.File, error) {
	// Root confines resolution; the caller checks regular type and inode before
	// and after opening. Unix additionally uses NOFOLLOW and NONBLOCK.
	return root.OpenFile(name, os.O_RDONLY, 0)
}
