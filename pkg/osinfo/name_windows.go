package osinfo

import (
	"fmt"
	"golang.org/x/sys/windows"
)

func Name() string {
	info := windows.RtlGetVersion()
	return fmt.Sprintf(
		"Windows %d.%d - Build %d",
		info.MajorVersion,
		info.MinorVersion,
		info.BuildNumber,
	)
}
