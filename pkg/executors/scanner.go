package executors

import (
	"io"
)

func ScanLines(r io.Reader, f func(line string) bool) error {
	// buffer := make([]byte, BufferSize)
	// log.Printf("Starting to read lines")

	// // Note that we do this manually rather than
	// // because we need to handle very long lines

	// for {
	// 	log.Printf("BBB")

	// 	_, err := r.Read(buffer)
	// 	if err != nil {
	// 		if err == io.EOF {
	// 			log.Printf("Encountered EOF")
	// 			break
	// 		}
	// 		return err
	// 	}

	// 	log.Printf("AAA")

	// 	continueReading := f(string(buffer))

	// 	if !continueReading {
	// 		break
	// 	}
	// }

	// log.Printf("Finished")
	return nil
}
