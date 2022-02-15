package shell

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
)

type Config struct {
	noPTY           bool
	Command         string
	Shell           *Shell
	TempStoragePath string
}

type Process struct {
	Config           Config
	StartedAt        int
	FinishedAt       int
	ExitCode         int
	OnStdoutCallback func(string)
	startMark        string
	endMark          string
	commandEndRegex  *regexp.Regexp
	cmdFilePath      string
	inputBuffer      []byte
	outputBuffer     *OutputBuffer
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
		cmdFilePath:     config.TempStoragePath + "/current-agent-cmd",
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

func (p *Process) Shell() *Shell {
	return p.Config.Shell
}

func (p *Process) Run() {
	if p.Config.noPTY {
		p.runWithoutPTY()
	} else {
		p.runWithPTY()
	}
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

	var stdoutBuf bytes.Buffer
	cmd := exec.Command("bash", "-c", instruction)
	cmd.Stdout = &stdoutBuf

	err = cmd.Start()
	if err != nil {
		log.Errorf("Error starting command: %v\n", err)
		p.ExitCode = 1
		return
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
	}

	p.OnStdoutCallback(stdoutBuf.String())
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

	_, err = p.Shell().Write(instruction)
	if err != nil {
		log.Errorf("Error writing instruction: %v", err)
		return
	}

	_ = p.scan()
}

func (p *Process) constructShellInstruction() string {
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

func (p *Process) loadCommand() error {
	//
	// Multiline commands don't work very well with the start/finish marker
	// scheme.  To circumvent this, we are storing the command in a file
	//

	// #nosec
	err := ioutil.WriteFile(p.cmdFilePath, []byte(p.Config.Command), 0644)
	if err != nil {
		// TODO: log something
		// e.Logger.LogCommandStarted(command)
		// e.Logger.LogCommandOutput(fmt.Sprintf("Failed to run command: %+v\n", err))

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
	n, err := p.Shell().Read(&buffer)
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
