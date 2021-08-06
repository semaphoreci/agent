package osinfo

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

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

func Hostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return ""
	}

	return hostname
}

func Arch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x86_64"

	case "386":
		return "x86"

	default:
		return runtime.GOARCH
	}
}

func namemac() string {
	o1, err := exec.Command("sw_vers", "-productName").Output()
	if err != nil {
		return ""
	}

	o2, err := exec.Command("sw_vers", "-productVersion").Output()
	if err != nil {
		return ""
	}

	o3, err := exec.Command("sw_vers", "-buildVersion").Output()
	if err != nil {
		return ""
	}

	productName := strings.TrimSpace(string(o1))
	productVersion := strings.TrimSpace(string(o2))
	buildVersion := strings.TrimSpace(string(o3))

	return fmt.Sprintf("%s %s %s", productName, productVersion, buildVersion)
}

func namelinux() string {
	out, err := exec.Command("cat", "/etc/os-release", "/etc/lsb-release").Output()
	if err != nil {
		return ""
	}

	// The format of the file looks like this (example)
	//
	// NAME="Ubuntu"
	// VERSION="14.04.5 LTS, Trusty Tahr"
	// ID=ubuntu
	// ID_LIKE=debian
	// PRETTY_NAME="Ubuntu 14.04.5 LTS"
	// VERSION_ID="14.04"
	// HOME_URL="http://www.ubuntu.com/"
	// SUPPORT_URL="http://help.ubuntu.com/"
	// BUG_REPORT_URL="http://bugs.launchpad.net/ubuntu/"
	//

	lines := strings.Split(string(out), "\n")

	findValue := func(key string) (string, bool) {
		for _, line := range lines {
			if strings.HasPrefix(line, key+"=") {
				name := strings.Split(line, key+"=")[1]

				// if the value is wrapped in quotes, remove the quotes
				if name[0] != '"' {
					return name, true
				} else {
					return strings.Split(name, "\"")[1], true
				}
			}
		}

		return "", false
	}

	if name, ok := findValue("PRETTY_NAME"); ok {
		return name
	}

	if name, ok := findValue("NAME"); ok {
		return name
	}

	return ""
}
