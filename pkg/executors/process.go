package executors

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
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
	restoreTtyMark  string
	commandEndRegex *regexp.Regexp

	tempStoragePath string
	cmdFilePath     string
}

func NewProcess(cmd string, tempStoragePath string, shell io.Writer, tty *os.File) *Process {
	startMark := "87d140552e404df69f6472729d2b2c1"
	endMark := "97d140552e404df69f6472729d2b2c2"
	restoreTtyMark := "97d140552e404df69f6472729d2b2c1"

	commandEndRegex := regexp.MustCompile(endMark + " " + `(\d)`)

	return &Process{
		Command:  cmd,
		ExitCode: 1,

		Shell: shell,
		TTY:   tty,

		startMark:      startMark,
		endMark:        endMark,
		restoreTtyMark: restoreTtyMark,

		commandEndRegex: commandEndRegex,
		tempStoragePath: tempStoragePath,
		cmdFilePath:     tempStoragePath + "/current-agent-cmd",
	}
}

func (p *Process) OnStdout(callback func(string)) {
	p.OnStdoutCallback = callback
}

func (p *Process) Run() {
	instruction := p.constructShellInstruction()

	p.StartedAt = int(time.Now().Unix())
	defer func() {
		p.FinishedAt = int(time.Now().Unix())
	}()

	p.send(instruction)
	p.scan()
}

func (p *Process) constructShellInstruction() string {
	//
	// A process is sending a complex instruction to the shell. The instruction
	// does the following:
	//
	//   1. display START marker
	//   2. execute the command file by sourcing it
	//   3. save the original exit status
	//   4. display the END marker with the exit status
	//   5. return the original exit status to the caller
	//
	template := `echo %s; source %s; AGENT_CMD_RESULT=$?; echo "%s $AGENT_CMD_RESULT"; echo \"exit $AGENT_CMD_RESULT\" | sh \n`

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

	p.Shell.Write([]byte(instruction))
}

func (p *Process) scan() {
	log.Println("Scan started")

	streamEvents := false

	ScanLines(p.TTY, func(line string) bool {
		log.Printf("(tty) %s\n", line)

		if strings.Contains(line, p.startMark) {
			log.Printf("Detected command start")
			streamEvents = true

			return true
		}

		if strings.Contains(line, p.endMark) {
			log.Printf("Detected command end")

			finalOutputPart := strings.Split(line, p.endMark)

			// if there is anything else other than the command end marker
			// print it to the user
			if finalOutputPart[0] != "" {
				p.OnStdoutCallback(finalOutputPart[0] + "\n")
			}

			streamEvents = false

			if match := p.commandEndRegex.FindStringSubmatch(line); len(match) == 2 {
				log.Printf("Parsing exit status succedded")

				exitCode, err := strconv.Atoi(match[1])
				if err != nil {
					log.Printf("Panic while parsing exit status, err: %+v", err)

					p.OnStdoutCallback("Failed to read command exit code\n")
				}

				log.Printf("Setting exit code to %d", exitCode)

				p.ExitCode = exitCode
			} else {
				log.Printf("Failed to parse exit status")

				p.OnStdoutCallback("Failed to read command exit code\n")
			}

			log.Printf("Stopping scanner")

			return false
		}

		if streamEvents {
			p.OnStdoutCallback(line + "\n")
		}

		return true
	})
}
