// +build windows

package shell

import (
	"unsafe"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/windows"
)

func (s *Shell) Setup() {
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
	s.windowsJobObject = uintptr(jobObject)
}

func (s *Shell) Terminate() error {
	log.Debugf("Terminating all processes assigned to job object %v", s.windowsJobObject)
	return windows.CloseHandle(windows.Handle(s.windowsJobObject))
}
