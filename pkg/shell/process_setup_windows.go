// +build windows

package shell

import (
	"unsafe"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/windows"
)

func (p *Process) setup() {
	p.SysProcAttr = &windows.SysProcAttr{
		CreationFlags: windows.CREATE_UNICODE_ENVIRONMENT | windows.CREATE_NEW_PROCESS_GROUP,
	}

	jobObject, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		log.Errorf("Error creating job object: %v", err)
		return
	}

	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}

	_, err = windows.SetInformationJobObject(
		jobObject,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	)

	if err != nil {
		log.Errorf("Error setting job object information: %v", err)
		return
	}

	log.Debugf("Successfully created job object: %v", jobObject)
	p.windowsJobObject = uintptr(jobObject)
}

func (p *Process) afterCreation() error {
	permissions := uint32(windows.PROCESS_QUERY_LIMITED_INFORMATION | windows.PROCESS_SET_QUOTA | windows.PROCESS_TERMINATE)
	processHandle, err := windows.OpenProcess(permissions, false, uint32(p.Pid))
	if err != nil {
		return err
	}

	defer windows.CloseHandle(processHandle)

	err = windows.AssignProcessToJobObject(windows.Handle(p.windowsJobObject), processHandle)
	if err != nil {
		return err
	}

	return nil
}

func (p *Process) Terminate() error {
	log.Debugf("Terminating all processes assigned to job object %v", p.windowsJobObject)
	return windows.CloseHandle(windows.Handle(p.windowsJobObject))
}
