package testsupport

import (
	"fmt"
	"io"
	"log"
	"os"
)

func SetupTestLogs() {
	// #nosec
	f, err := os.OpenFile("/tmp/test.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0777)
	if err != nil {
		fmt.Printf("error opening file: %v", err)
		panic("can't open log file")
	}

	wrt := io.MultiWriter(os.Stdout, f)
	log.SetOutput(wrt)
	log.SetFlags(log.Ldate | log.Lmicroseconds | log.Lshortfile)
}
