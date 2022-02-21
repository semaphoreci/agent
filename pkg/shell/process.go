package shell

import (
	"bytes"
	"flag"
	"fmt"
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
const WINDOWS_BATCH_SCRIPT = `
@echo off
%s
SET SEMAPHORE_AGENT_CURRENT_CMD_EXIT_STATUS=%%ERRORLEVEL%%
SET SEMAPHORE_AGENT_CURRENT_DIR=%%CD%%
SET > "%s.env.after"
EXIT \B %%SEMAPHORE_AGENT_CURRENT_CMD_EXIT_STATUS%%
`

type Config struct {
	noPTY     bool
	Command   string
	Shell     *Shell
	ExtraVars *Environment
}

type Process struct {
	Config           Config
	StartedAt        int
	FinishedAt       int
	ExitCode         int
	OnStdoutCallback func(string)
	Pid              int
	startMark        string
	endMark          string
	commandEndRegex  *regexp.Regexp
	cmdFilePath      string
	inputBuffer      []byte
	outputBuffer     *OutputBuffer
	SysProcAttr      *syscall.SysProcAttr

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
		Config:          config,
		ExitCode:        1,
		startMark:       startMark,
		endMark:         endMark,
		commandEndRegex: commandEndRegex,
		cmdFilePath:     osinfo.FormTempDirPath("current-agent-cmd"),
		outputBuffer:    NewOutputBuffer(),
	}
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
	if !p.Config.noPTY {
		p.runWithPTY()
		return
	}

	/*
	 * If we are not using a PTY, we need to keep track of
	 * environment variables and the current working directory.
	 */
	p.setup()
	p.runWithoutPTY()

	after, _ := EnvFromDump(fmt.Sprintf("%s.env.after", p.cmdFilePath))
	newCwd, _ := after.Get("SEMAPHORE_AGENT_CURRENT_DIR")
	p.Config.Shell.Chdir(newCwd)

	// Remove variables we added
	after.Remove("SEMAPHORE_AGENT_CURRENT_DIR")
	after.Remove("SEMAPHORE_AGENT_CURRENT_CMD_EXIT_STATUS")

	p.Config.Shell.UpdateEnvironment(after)
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

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		// #nosec
		cmd = exec.Command("C:\\Windows\\System32\\CMD.exe", "/S", "/C", instruction)
	} else {
		// #nosec
		cmd = exec.Command("bash", "-c", instruction)
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	cmd.Dir = p.Config.Shell.Cwd

	if p.Config.Shell.Env != nil {
		cmd.Env = append(os.Environ(), p.Config.Shell.Env.ToArray()...)
	}

	if p.Config.ExtraVars != nil {
		// #nosec
		cmd.Env = append(cmd.Env, p.Config.ExtraVars.ToArray()...)
	}

	cmd.SysProcAttr = p.SysProcAttr

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

	waitResult := cmd.Wait()
	if waitResult != nil {
		if err, ok := waitResult.(*exec.ExitError); ok {
			if s, ok := err.Sys().(syscall.WaitStatus); ok {
				p.ExitCode = s.ExitStatus()
			} else {
				log.Error("Unimplemented for system where exec.ExitError.Sys() is not syscall.WaitStatus.")
			}
		} else {
			log.Errorf("Unexpected error type %T", waitResult)
		}
	} else {
		p.ExitCode = 0
	}

	p.OnStdoutCallback(stdoutBuf.String())
	p.OnStdoutCallback(stderrBuf.String())
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

	_, err = p.Config.Shell.Write(instruction)
	if err != nil {
		log.Errorf("Error writing instruction: %v", err)
		return
	}

	_ = p.scan()
}

func (p *Process) constructShellInstruction() string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf(`%s.bat`, p.cmdFilePath)
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

	return fmt.Sprintf(template, p.startMark, p.cmdFilePath, p.endMark)
}

/*
 * Multiline commands don't work very well with the start/finish marker.
 * scheme. To circumvent this, we are storing the command in a file.
 */
func (p *Process) loadCommand() error {
	if runtime.GOOS != "windows" {
		return p.writeCommand(p.cmdFilePath, p.Config.Command)
	}

	cmdFilePath := fmt.Sprintf("%s.bat", p.cmdFilePath)
	command := fmt.Sprintf(WINDOWS_BATCH_SCRIPT, buildCommand(p.Config.Command), p.cmdFilePath)
	return p.writeCommand(cmdFilePath, command)
}

func (p *Process) writeCommand(cmdFilePath, command string) error {
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
	n, err := p.Config.Shell.Read(&buffer)
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
