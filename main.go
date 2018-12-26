package main

import (
	"os"
)

func main() {
	job, err := NewJobFromYaml(os.Args[1])

	if err != nil {
		panic(err)
	}

	job.Run()
}
