package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Printf(os.Args[1])
	job, err := NewJobFromYaml(os.Args[1])

	if err != nil {
		panic(err)
	}

	job.Run()
}
