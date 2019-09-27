package executors

struct CommandOutputReader {
	eventHandler EventHandler
}

func NewCommandOutputReader(eventHandler EventHandler) {
	return &CommandOutputReader{
		eventHandler EventHandler
	}
}

func (r *Reader) Scan(
	log.Println("Scan started")

	err = ScanLines(e.tty, func(line string) bool {
		log.Printf("(tty) %s\n", line)

		if strings.Contains(line, startMark) {
			log.Printf("Detected command start")
			streamEvents = true

			callback(NewCommandStartedEvent(command))

			return true
		}

		if strings.Contains(line, finishMark) {
			log.Printf("Detected command end")

			finalOutputPart := strings.Split(line, finishMark)

			// if there is anything else other than the command end marker
			// print it to the user
			if finalOutputPart[0] != "" {
				callback(NewCommandOutputEvent(finalOutputPart[0] + "\n"))
			}

			streamEvents = false

			if match := commandEndRegex.FindStringSubmatch(line); len(match) == 2 {
				log.Printf("Parsing exit status succedded")

				exitCode, err = strconv.Atoi(match[1])

				if err != nil {
					log.Printf("Panic while parsing exit status, err: %+v", err)

					callback(NewCommandOutputEvent("Failed to read command exit code\n"))
				}

				log.Printf("Setting exit code to %d", exitCode)
			} else {
				log.Printf("Failed to parse exit status")

				exitCode = 1
				callback(NewCommandOutputEvent("Failed to read command exit code\n"))
			}

			log.Printf("Stopping scanner")
			return false
		}

		if streamEvents {
			callback(NewCommandOutputEvent(line + "\n"))
		}

		return true
	})
}
