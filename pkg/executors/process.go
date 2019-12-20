package executors

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Process struct {
	Command string
	Shell   io.Writer
	TTY     *os.File

	StartedAt  int
	FinishedAt int
	ExitCode   int

	OnStdoutCallback func(string)

	startMark       string
	endMark         string
	commandEndRegex *regexp.Regexp

	tempStoragePath string
	cmdFilePath     string

	inputBuffer  []byte
	outputBuffer []byte

	lastStream time.Time
}

func randomMagicMark() string {
	return fmt.Sprintf("949556c7-%d", time.Now().Unix())
}

func NewProcess(cmd string, tempStoragePath string, shell io.Writer, tty *os.File) *Process {
	startMark := randomMagicMark() + "-start"
	endMark := randomMagicMark() + "-end"

	commandEndRegex := regexp.MustCompile(endMark + " " + `(\d)` + "[\r\n]+")

	return &Process{
		Command:  cmd,
		ExitCode: 1,

		Shell: shell,
		TTY:   tty,

		startMark: startMark,
		endMark:   endMark,

		commandEndRegex: commandEndRegex,
		tempStoragePath: tempStoragePath,
		cmdFilePath:     tempStoragePath + "/current-agent-cmd",
		lastStream:      time.Now(),
	}
}

func (p *Process) OnStdout(callback func(string)) {
	p.OnStdoutCallback = callback
}

func (p *Process) StreamToStdout() {
	for len(p.outputBuffer) > 100 || time.Now().Sub(p.lastStream) > 100*time.Millisecond {
		l := 100

		if len(p.outputBuffer) < l {
			l = len(p.outputBuffer)
		}

		output := make([]byte, l)
		copy(output, p.outputBuffer[0:l])
		p.outputBuffer = p.outputBuffer[l:]

		out := strings.Replace(string(output), "\r\n", "\n", -1)

		log.Printf("Stream to stdout: %#v", out)
		p.OnStdoutCallback(out)

		p.lastStream = time.Now()
	}
}

func (p *Process) flushOutputBuffer() {
	for {
		p.StreamToStdout()

		if len(p.outputBuffer) == 0 {
			break
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

	p.outputBuffer = append(p.outputBuffer, p.inputBuffer[0:index]...)
	p.inputBuffer = p.inputBuffer[index:]
}

func (p *Process) Run() {
	instruction := p.constructShellInstruction()

	p.StartedAt = int(time.Now().Unix())
	defer func() {
		p.FinishedAt = int(time.Now().Unix())
	}()

	err := p.loadCommand()
	if err != nil {
		log.Printf("err: %v", err)
		return
	}

	p.send(instruction)
	p.scan()
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

	err := ioutil.WriteFile(p.cmdFilePath, []byte(p.Command), 0644)
	if err != nil {
		// TODO: log something
		// e.Logger.LogCommandStarted(command)
		// e.Logger.LogCommandOutput(fmt.Sprintf("Failed to run command: %+v\n", err))

		return err
	}

	return nil
}

func (p *Process) send(instruction string) {
	log.Printf("Sending Instruction: %s", instruction)

	p.Shell.Write([]byte(instruction + "\n"))
}

func (p *Process) readBufferSize() int {
	if flag.Lookup("test.v") == nil {
		return 100
	} else {
		// simulating the worst kind of baud rate
		// random in size, and possibly very short

		// The implementation needs to handle everything.
		rand.Seed(time.Now().UnixNano())

		min := 1
		max := 20

		return rand.Intn(max-min) + min
	}
}

//
// Read state from shell into the outputBuffer
//
func (p *Process) read() error {
	buffer := make([]byte, p.readBufferSize())

	log.Println("Reading started")
	n, err := p.TTY.Read(buffer)
	if err != nil {
		log.Println("Error while reading from the tty")
		return err
	}

	p.inputBuffer = append(p.inputBuffer, buffer[0:n]...)
	log.Printf("reading data from shell. Input buffer: %#v", string(p.inputBuffer))

	return nil
}

func (p *Process) waitForStartMarker() error {
	log.Println("Waiting for start marker", p.startMark)

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

	log.Println("Start marker found", p.startMark)

	return nil
}

func (p *Process) endMarkerHeaderIndex() int {
	return strings.Index(string(p.inputBuffer), "\001")
}

func (p *Process) scan() error {
	log.Println("Scan started")

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

			log.Println("Start of end marker detected, entering buffering mode.")

			if match := p.commandEndRegex.FindStringSubmatch(string(p.inputBuffer)); len(match) == 2 {
				log.Println("End marker detected. Exit code: ", match[1])

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
			return err
		}
	}

	p.flushOutputBuffer()

	log.Println("Command output finished")
	log.Println("Parsing exit code", exitCode)

	code, err := strconv.Atoi(exitCode)
	if err != nil {
		log.Printf("Error while parsing exit code, err: %v", err)

		return err
	}

	log.Printf("Parsing exit code fininished %d", code)
	p.ExitCode = code

	return nil
}
