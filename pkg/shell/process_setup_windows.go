// +build windows

package shell

import (
	"golang.org/x/sys/windows"
)

func (p *Process) setup() {
	p.SysProcAttr = &windows.SysProcAttr{
		CreationFlags: windows.CREATE_UNICODE_ENVIRONMENT | windows.CREATE_NEW_PROCESS_GROUP,
	}
}

func (p *Process) afterCreation(jobObject uintptr) error {
	permissions := uint32(windows.PROCESS_QUERY_LIMITED_INFORMATION | windows.PROCESS_SET_QUOTA | windows.PROCESS_TERMINATE)
	processHandle, err := windows.OpenProcess(permissions, false, uint32(p.Pid))
	if err != nil {
		return err
	}

	defer windows.CloseHandle(processHandle)

	err = windows.AssignProcessToJobObject(windows.Handle(jobObject), processHandle)
	if err != nil {
		return err
	}

	return nil
}
