package executors

import (
	"bufio"
	"io"
	"log"
)

func ScanLines(r io.Reader, f func(line string) bool) error {
	var reader = bufio.NewReader(r)
	var appending []byte

	log.Printf("[LineScanner] Starting to read lines")

	// Note that we do this manually rather than
	// because we need to handle very long lines

	for {
		line, isPrefix, err := reader.ReadLine()
		if err != nil {
			if err == io.EOF {
				log.Printf("[LineScanner] Encountered EOF")
				break
			}
			return err
		}

		// If isPrefix is true, that means we've got a really
		// long line incoming, and we'll keep appending to it
		// until isPrefix is false (which means the long line
		// has ended.
		if isPrefix && appending == nil {
			log.Printf("[LineScanner] Line is too long to read, going to buffer it until it finishes")

			// bufio.ReadLine returns a slice which is only valid until the next invocation
			// since it points to its own internal buffer array. To accumulate the entire
			// result we make a copy of the first prefix, and ensure there is spare capacity
			// for future appends to minimize the need for resizing on append.
			appending = make([]byte, len(line), (cap(line))*2)
			copy(appending, line)

			continue
		}

		// Should we be appending?
		if appending != nil {
			appending = append(appending, line...)

			// No more isPrefix! Line is finished!
			if !isPrefix {
				log.Printf("[LineScanner] Finished buffering long line")
				line = appending

				// Reset appending back to nil
				appending = nil
			} else {
				continue
			}
		}

		// Write to the handler function
		continueReading := f(string(line))

		if !continueReading {
			break
		}
	}

	log.Printf("[LineScanner] Finished")
	return nil
}
