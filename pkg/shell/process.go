package shell

import (
	"flag"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
)

/*
 * Windows does not support a PTY yet. To allow changing directories,
 * and setting/unsetting environment variables, we need to keep track
 * of the environment on every command executed. We do that by
 * getting the whole environment after a command is executed and
 * updating our shell with it.
 */
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
	OnOutput    func(string)
}

type Process struct {
	Command         string
	Shell           *Shell
	StoragePath     string
	StartedAt       int
	FinishedAt      int
	ExitCode        int
	Pid             int
	startMark       string
	endMark         string
	commandEndRegex *regexp.Regexp
	inputBuffer     []byte
	outputBuffer    *OutputBuffer
	SysProcAttr     *syscall.SysProcAttr
}

func randomMagicMark() string {
	return fmt.Sprintf("949556c7-%d", time.Now().Unix())
}

func NewProcess(config Config) *Process {
	startMark := randomMagicMark() + "-start"
	endMark := randomMagicMark() + "-end"
	commandEndRegex := regexp.MustCompile(endMark + " " + `(\d+)` + "[\r\n]+")
	outputBuffer, _ := NewOutputBuffer(config.OnOutput)

	return &Process{
		Shell:           config.Shell,
		StoragePath:     config.StoragePath,
		Command:         config.Command,
		ExitCode:        1,
		startMark:       startMark,
		endMark:         endMark,
		commandEndRegex: commandEndRegex,
		outputBuffer:    outputBuffer,
	}
}

func (p *Process) CmdFilePath() string {
	return filepath.Join(p.StoragePath, "current-agent-cmd")
}

func (p *Process) EnvironmentFilePath() string {
	return fmt.Sprintf("%s.env.after", p.CmdFilePath())
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

	/*
	 * If the agent is running in an non-windows environment,
	 * we use a PTY session to run commands.
	 */
	if runtime.GOOS != "windows" {
		p.runWithPTY(instruction)
		return
	}

	// In windows, so no PTY support.
	p.setup()
	p.runWithoutPTY(instruction)

	/*
	 * If we are not using a PTY, we need to keep track of shell "state" ourselves.
	 * We use a file with all the environment variables available after the command
	 * is executed. From that file, we can update our shell "state".
	 */
	after, err := CreateEnvironmentFromFile(p.EnvironmentFilePath())
	if err != nil {
		log.Errorf("Error creating environment from file %s: %v\n", p.EnvironmentFilePath(), err)
		return
	}

	/*
	 * CMD.exe does not have an environment variable such as $PWD,
	 * so we use a custom one to get the current working directory
	 * after a command is executed.
	 */
	newCwd, exists := after.Get("SEMAPHORE_AGENT_CURRENT_DIR")
	if exists {
		p.Shell.Chdir(newCwd)
	}

	/*
	 * We use two custom environment variables to track
	 * things we need, but we don't want to mess the environment
	 * so we remove them before updating our shell state.
	 */
	after.Remove("SEMAPHORE_AGENT_CURRENT_DIR")
	after.Remove("SEMAPHORE_AGENT_CURRENT_CMD_EXIT_STATUS")
	p.Shell.UpdateEnvironment(after)
}

// TODO: start output buffer flushing here
func (p *Process) runWithoutPTY(instruction string) {
	cmd, reader, writer := p.buildNonPTYCommand(instruction)
	err := cmd.Start()
	if err != nil {
		log.Errorf("Error starting command: %v\n", err)
		p.ExitCode = 1
		return
	}

	/*
	 * In Windows, we need to assign the process created by the command
	 * with the job object handle we created for it earlier,
	 * for process termination purposes.
	 */
	p.Pid = cmd.Process.Pid
	err = p.afterCreation(p.Shell.windowsJobObject)
	if err != nil {
		log.Errorf("Process after creation procedure failed: %v", err)
	}

	/*
	 * Start reading the command's output and wait until it finishes.
	 */
	done := make(chan bool, 1)
	go p.readNonPTY(reader, done)
	waitResult := cmd.Wait()

	/*
	 * Command is done, We close our output writer
	 * so our output reader knows the command is over.
	 */
	err = writer.Close()
	if err != nil {
		log.Errorf("Error closing writer: %v", err)
	}

	/*
	 * Let's wait for the reader to finish, just to make sure
	 * we don't leave any goroutines hanging around.
	 */
	log.Debug("Waiting for reading to finish")
	<-done

	/*
	 * The command was successful, so we just return.
	 */
	if waitResult == nil {
		p.ExitCode = 0
		return
	}

	/*
	 * The command returned an error, so we need to figure out the exit code from it.
	 * If we can't figure it out, we just use 1 and carry on.
	 */
	if err, ok := waitResult.(*exec.ExitError); ok {
		if s, ok := err.Sys().(syscall.WaitStatus); ok {
			p.ExitCode = s.ExitStatus()
		} else {
			log.Errorf("Could not cast *exec.ExitError to syscall.WaitStatus: %v\n", err)
			p.ExitCode = 1
		}
	} else {
		log.Errorf("Unexpected %T returned after Wait(): %v", waitResult, waitResult)
		p.ExitCode = 1
	}
}

func (p *Process) buildNonPTYCommand(instruction string) (*exec.Cmd, *io.PipeReader, *io.PipeWriter) {
	args := append(p.Shell.Args, instruction)

	// #nosec
	cmd := exec.Command(p.Shell.Executable, args...)
	cmd.Dir = p.Shell.Cwd
	cmd.SysProcAttr = p.SysProcAttr

	if p.Shell.Env != nil {
		cmd.Env = append(os.Environ(), p.Shell.Env.ToSlice()...)
	}

	reader, writer := io.Pipe()
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd, reader, writer
}

func (p *Process) runWithPTY(instruction string) {
	_, err := p.Shell.Write(instruction)
	if err != nil {
		log.Errorf("Error writing instruction: %v", err)
		return
	}

	_ = p.scan()
}

func (p *Process) constructShellInstruction() string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf(`%s.ps1`, p.CmdFilePath())
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

	cmdFilePath := fmt.Sprintf("%s.ps1", p.CmdFilePath())
	command := fmt.Sprintf(WindowsPwshScript, p.Command, p.CmdFilePath())
	return p.writeCommandToFile(cmdFilePath, command)
}

func (p *Process) writeCommandToFile(cmdFilePath, command string) error {
	// #nosec
	file, err := os.OpenFile(cmdFilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}

	/*
	 * UTF8 files without a BOM containing non-ASCII characters may break in Windows PowerShell,
	 * since it misinterprets it as being encoded in the legacy "ANSI" codepage.
	 * Since we need to support non-ASCII characters, we need a UTF-8 file with a BOM.
	 * See: https://docs.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_character_encoding
	 */
	if runtime.GOOS == "windows" {
		_, err = file.Write([]byte{0xEF, 0xBB, 0xBF})
		if err != nil {
			_ = file.Close()
			return err
		}
	}

	_, err = file.Write([]byte(command))
	if err != nil {
		_ = file.Close()
		return err
	}

	return file.Close()
}

// TODO: if we concurrently read what's in the output buffer, we probably don't need to worry about this.
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

func (p *Process) readNonPTY(reader *io.PipeReader, done chan bool) {
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

		if err == io.EOF {
			log.Debug("Finished reading")
			p.outputBuffer.Close()
			break
		}
	}

	done <- true
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

		err := p.read()
		if err != nil {
			// Reading failed. The most likely cause is that the bash process
			// died. For example, running an `exit 1` command has killed it.
			// Flushing all remaining data in the buffer and exiting.
			p.outputBuffer.Close()

			return err
		}
	}

	p.outputBuffer.Close()

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
