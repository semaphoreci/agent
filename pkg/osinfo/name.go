// +build !windows

package osinfo

import "runtime"

func Name() string {
	switch runtime.GOOS {
	case "linux":
		return namelinux()
	case "darwin":
		return namemac()
	default:
		// TODO handle other OSes
		return ""
	}
}
