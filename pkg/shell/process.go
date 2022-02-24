package shell

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/semaphoreci/agent/pkg/osinfo"
	log "github.com/sirupsen/logrus"
)

/*
 * Windows does not support a PTY yet. To allow changing directories,
 * and setting/unsetting environment variables, we need to keep track
 * of the environment on every command executed. We do that by
 * getting the whole environment after a command is executed and
 * updating our shell with it.
 */
const WindowsBatchScript = `
@echo off
%s
SET SEMAPHORE_AGENT_CURRENT_CMD_EXIT_STATUS=%%ERRORLEVEL%%
SET SEMAPHORE_AGENT_CURRENT_DIR=%%CD%%
SET > "%s.env.after"
EXIT \B %%SEMAPHORE_AGENT_CURRENT_CMD_EXIT_STATUS%%
`

const WindowsPwshScript = `
$ErrorActionPreference = "STOP"
%s
if ($LASTEXITCODE -eq $null) {$Env:SEMAPHORE_AGENT_CURRENT_CMD_EXIT_STATUS = 0} else {$Env:SEMAPHORE_AGENT_CURRENT_CMD_EXIT_STATUS = $LASTEXITCODE}
$Env:SEMAPHORE_AGENT_CURRENT_DIR = $PWD | Select-Object -ExpandProperty Path
Get-ChildItem Env: | Foreach-Object {"$($_.Name)=$($_.Value)"} | Set-Content "%s.env.after"
exit $Env:SEMAPHORE_AGENT_CURRENT_CMD_EXIT_STATUS
`

type Config struct {
	Shell       *Shell
	StoragePath string
	Command     string
}

type Process struct {
	Command          string
	Shell            *Shell
	StoragePath      string
	StartedAt        int
	FinishedAt       int
	ExitCode         int
	OnStdoutCallback func(string)
	Pid              int
	startMark        string
	endMark          string
	commandEndRegex  *regexp.Regexp
	inputBuffer      []byte
	outputBuffer     *OutputBuffer
	SysProcAttr      *syscall.SysProcAttr

	/*
	 * A job object handle used to interrupt the command
	 * process in case of a stop request.
	 */
	windowsJobObject uintptr
}

func randomMagicMark() string {
	return fmt.Sprintf("949556c7-%d", time.Now().Unix())
}

func NewProcess(config Config) *Process {
	startMark := randomMagicMark() + "-start"
	endMark := randomMagicMark() + "-end"

	commandEndRegex := regexp.MustCompile(endMark + " " + `(\d+)` + "[\r\n]+")

	return &Process{
		Shell:           config.Shell,
		StoragePath:     config.StoragePath,
		Command:         config.Command,
		ExitCode:        1,
		startMark:       startMark,
		endMark:         endMark,
		commandEndRegex: commandEndRegex,
		outputBuffer:    NewOutputBuffer(),
	}
}

func (p *Process) CmdFilePath() string {
	return osinfo.FormDirPath(p.StoragePath, "current-agent-cmd")
}

func (p *Process) EnvironmentFilePath() string {
	return fmt.Sprintf("%s.env.after", p.CmdFilePath())
}

func (p *Process) OnStdout(callback func(string)) {
	p.OnStdoutCallback = callback
}

func (p *Process) StreamToStdout() {
	for {
		data, ok := p.outputBuffer.Flush()
		if !ok {
			break
		}

		log.Debugf("Stream to stdout: %#v", data)

		p.OnStdoutCallback(data)
	}
}

func (p *Process) flushOutputBuffer() {
	for !p.outputBuffer.IsEmpty() {
		p.StreamToStdout()

		if !p.outputBuffer.IsEmpty() {
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func (p *Process) flushInputAll() {
	p.flushInputBufferTill(len(p.inputBuffer))
}

func (p *Process) flushInputBufferTill(index int) {
	if index == 0 {
		return
	}

	data := p.inputBuffer[0:index]
	p.inputBuffer = p.inputBuffer[index:]

	p.outputBuffer.Append(data)
}

func (p *Process) Run() {
	/*
	 * If the agent is running in an non-windows environment,
	 * we use a PTY session to run commands.
	 */
	if runtime.GOOS != "windows" {
		p.runWithPTY()
		return
	}

	/*
	 * WHen running in windows, we need to create a job object handle,
	 * which will be assigned a process after the command's process starts.
	 * This is needed in order to properly terminate the processes in case
	 * the job is stopped.
	 */
	p.setup()

	// In windows, so no PTY support.
	p.runWithoutPTY()

	/*
	 * If we are not using a PTY, we need to keep track of shell "state" ourselves.
	 * We use a file with all the environment variables available after the command
	 * is executed. From that file, we can update our shell "state".
	 */
	after, _ := CreateEnvironmentFromFile(p.EnvironmentFilePath())

	/*
	 * CMD.exe does not have an environment variable such as $PWD,
	 * so we use a custom one to get the current working directory
	 * after a command is executed.
	 */
	newCwd, _ := after.Get("SEMAPHORE_AGENT_CURRENT_DIR")
	p.Shell.Chdir(newCwd)

	/*
	 * We use two custom environment variables to track
	 * things we need, but we don't want to mess the environment
	 * so we remove them before updating our shell state.
	 */
	after.Remove("SEMAPHORE_AGENT_CURRENT_DIR")
	after.Remove("SEMAPHORE_AGENT_CURRENT_CMD_EXIT_STATUS")
	p.Shell.UpdateEnvironment(after)
}

func (p *Process) runWithoutPTY() {
	instruction := p.constructShellInstruction()
	p.StartedAt = int(time.Now().Unix())
	defer func() {
		p.FinishedAt = int(time.Now().Unix())
	}()

	err := p.loadCommand()
	if err != nil {
		log.Errorf("Err: %v", err)
		return
	}

	shell := p.findShell()
	command := shell[0]
	args := shell[1:]
	args = append(args, instruction)

	// #nosec
	cmd := exec.Command(command, args...)
	cmd.Dir = p.Shell.Cwd
	cmd.SysProcAttr = p.SysProcAttr

	if p.Shell.Env != nil {
		cmd.Env = append(os.Environ(), p.Shell.Env.ToArray()...)
	}

	reader, writer := io.Pipe()
	cmd.Stdout = writer
	cmd.Stderr = writer

	err = cmd.Start()
	if err != nil {
		log.Errorf("Error starting command: %v\n", err)
		p.ExitCode = 1
		return
	}

	p.Pid = cmd.Process.Pid
	err = p.afterCreation()
	if err != nil {
		log.Errorf("Process after creation procedure failed: %v", err)
	}

	done := make(chan bool, 1)
	go func() {
		for {
			log.Debug("Reading started")
			buffer := make([]byte, p.readBufferSize())
			n, err := reader.Read(buffer)
			if err != nil && err != io.EOF {
				log.Errorf("Error while reading. Error: %v", err)
			}

			p.inputBuffer = append(p.inputBuffer, buffer[0:n]...)
			log.Debugf("reading data from command. Input buffer: %#v", string(p.inputBuffer))
			p.flushInputAll()
			p.StreamToStdout()

			if err == io.EOF {
				log.Debug("Finished reading")
				p.flushOutputBuffer()
				break
			}
		}

		done <- true
	}()

	waitResult := cmd.Wait()

	err = writer.Close()
	if err != nil {
		log.Errorf("Error closing writer: %v", err)
	}

	log.Debug("Waiting for reading to finish...")
	<-done

	if waitResult == nil {
		p.ExitCode = 0
		return
	}

	if err, ok := waitResult.(*exec.ExitError); ok {
		if s, ok := err.Sys().(syscall.WaitStatus); ok {
			p.ExitCode = s.ExitStatus()
		} else {
			log.Error("Unimplemented for system where exec.ExitError.Sys() is not syscall.WaitStatus.")
		}
	} else {
		log.Errorf("Unexpected error type %T: %v", waitResult, waitResult)
	}
}

func (p *Process) findShell() []string {
	if runtime.GOOS == "windows" {
		shell := os.Getenv("SEMAPHORE_AGENT_SHELL")
		if shell == "powershell" {
			return []string{
				"C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\powershell.exe",
				"-NoProfile",
				"-NonInteractive",
			}
		}

		return []string{
			"C:\\Windows\\System32\\CMD.exe",
			"/S",
			"/C",
		}
	}

	return []string{"bash", "-c"}
}

func (p *Process) runWithPTY() {
	instruction := p.constructShellInstruction()

	p.StartedAt = int(time.Now().Unix())
	defer func() {
		p.FinishedAt = int(time.Now().Unix())
	}()

	err := p.loadCommand()
	if err != nil {
		log.Errorf("Err: %v", err)
		return
	}

	_, err = p.Shell.Write(instruction)
	if err != nil {
		log.Errorf("Error writing instruction: %v", err)
		return
	}

	_ = p.scan()
}

func (p *Process) constructShellInstruction() string {
	if runtime.GOOS == "windows" {
		shell := os.Getenv("SEMAPHORE_AGENT_SHELL")
		if shell == "powershell" {
			return fmt.Sprintf(`%s.ps1`, p.CmdFilePath())
		}

		// CMD.exe is the default on Windows
		return fmt.Sprintf(`%s.bat`, p.CmdFilePath())
	}

	//
	// A process is sending a complex instruction to the shell. The instruction
	// does the following:
	//
	//   1. display magic-header and the START marker
	//   2. execute the command file by sourcing it
	//   3. save the original exit status
	//   4. display magic-header, the end marker, and the command's exit status
	//   5. return the original exit status to the caller
	//
	template := `echo -e "\001 %s"; source %s; AGENT_CMD_RESULT=$?; echo -e "\001 %s $AGENT_CMD_RESULT"; echo "exit $AGENT_CMD_RESULT" | sh`

	return fmt.Sprintf(template, p.startMark, p.CmdFilePath(), p.endMark)
}

/*
 * Multiline commands don't work very well with the start/finish marker.
 * scheme. To circumvent this, we are storing the command in a file.
 */
func (p *Process) loadCommand() error {
	if runtime.GOOS != "windows" {
		return p.writeCommandToFile(p.CmdFilePath(), p.Command)
	}

	shell := os.Getenv("SEMAPHORE_AGENT_SHELL")
	if shell == "powershell" {
		cmdFilePath := fmt.Sprintf("%s.ps1", p.CmdFilePath())
		command := fmt.Sprintf(WindowsPwshScript, buildCommand(p.Command), p.CmdFilePath())
		return p.writeCommandToFile(cmdFilePath, command)
	}

	cmdFilePath := fmt.Sprintf("%s.bat", p.CmdFilePath())
	command := fmt.Sprintf(WindowsBatchScript, buildCommand(p.Command), p.CmdFilePath())
	return p.writeCommandToFile(cmdFilePath, command)
}

func (p *Process) writeCommandToFile(cmdFilePath, command string) error {
	// #nosec
	err := ioutil.WriteFile(cmdFilePath, []byte(command), 0644)
	if err != nil {
		return err
	}

	return nil
}

func (p *Process) readBufferSize() int {
	if flag.Lookup("test.v") == nil {
		return 100
	}

	// simulating the worst kind of baud rate
	// random in size, and possibly very short

	// The implementation needs to handle everything.
	rand.Seed(time.Now().UnixNano())

	min := 1
	max := 20

	// #nosec
	return rand.Intn(max-min) + min
}

//
// Read state from shell into the inputBuffer
//
func (p *Process) read() error {
	buffer := make([]byte, p.readBufferSize())

	log.Debug("Reading started")
	n, err := p.Shell.Read(&buffer)
	if err != nil {
		log.Errorf("Error while reading from the tty. Error: '%s'.", err.Error())
		return err
	}

	p.inputBuffer = append(p.inputBuffer, buffer[0:n]...)
	log.Debugf("reading data from shell. Input buffer: %#v", string(p.inputBuffer))

	return nil
}

func (p *Process) waitForStartMarker() error {
	log.Debugf("Waiting for start marker %s", p.startMark)

	//
	// Fill the output buffer, until the start marker appears
	//
	for {
		err := p.read()
		if err != nil {
			return err
		}

		//
		// If the inputBuffer has a start marker, the wait is done
		//
		index := strings.Index(string(p.inputBuffer), p.startMark+"\r\n")

		if index >= 0 {
			//
			// Cut everything from the buffer before the marker
			// Example:
			//
			// buffer before: some test <***marker**> rest of the test
			// buffer after :  rest of the test
			//

			p.inputBuffer = p.inputBuffer[index+len(p.startMark)+2 : len(p.inputBuffer)]

			break
		}
	}

	log.Debugf("Start marker found %s", p.startMark)

	return nil
}

func (p *Process) endMarkerHeaderIndex() int {
	return strings.Index(string(p.inputBuffer), "\001")
}

func (p *Process) scan() error {
	log.Debug("Scan started")

	err := p.waitForStartMarker()
	if err != nil {
		return err
	}

	exitCode := ""

	for {
		if index := p.endMarkerHeaderIndex(); index >= 0 {
			if index > 0 {
				// publish everything until the end mark
				p.flushInputBufferTill(index)
			}

			log.Debug("Start of end marker detected, entering buffering mode.")

			if match := p.commandEndRegex.FindStringSubmatch(string(p.inputBuffer)); len(match) == 2 {
				log.Debug("End marker detected. Exit code: ", match[1])

				exitCode = match[1]
				break
			}

			//
			// The buffer is much longer than the end mark, at least by 10
			// characters.
			//
			// If it is not matching the full end mark, it is safe to dump.
			//
			if len(p.inputBuffer) >= len(p.endMark)+10 {
				p.flushInputAll()
			}
		} else {
			p.flushInputAll()
		}

		p.StreamToStdout()

		err := p.read()
		if err != nil {
			// Reading failed. The most likely cause is that the bash process
			// died. For example, running an `exit 1` command has killed it.

			// Flushing all remaining data in the buffer and exiting.
			p.flushOutputBuffer()

			return err
		}
	}

	p.flushOutputBuffer()

	log.Debug("Command output finished")
	log.Debugf("Parsing exit code %s", exitCode)

	code, err := strconv.Atoi(exitCode)
	if err != nil {
		log.Errorf("Error while parsing exit code, err: %v", err)

		return err
	}

	log.Debugf("Parsing exit code finished %d", code)
	p.ExitCode = code

	return nil
}

func buildCommand(fullCommand string) string {
	commands := strings.Split(fullCommand, "\n")
	finalCommand := []string{}

	for _, command := range commands {
		parts := strings.Fields(strings.TrimSpace(command))
		if len(parts) < 1 {
			finalCommand = append(finalCommand, command)
			continue
		}

		firstPart := strings.ToLower(parts[0])
		if strings.HasSuffix(firstPart, ".bat") || strings.HasSuffix(firstPart, ".cmd") {
			finalCommand = append(finalCommand, fmt.Sprintf("CALL %s", command))
		} else {
			finalCommand = append(finalCommand, command)
		}
	}

	return strings.Join(finalCommand, "\n")
}
