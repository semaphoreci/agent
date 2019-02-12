package main

import (
	"fmt"
	"os"

	server "github.com/semaphoreci/agent/pkg/server"
)

var VERSION = "dev"

func main() {
	action := os.Args[1]

	switch action {
	case "serve":
		server.NewServer("0.0.0.0", 8000, VERSION).Serve()

	case "run":
		// job, err := NewJobFromYaml(os.Args[2])

		// if err != nil {
		// 	panic(err)
		// }

		// job.Run()

	case "version":
		fmt.Println(VERSION)
	}
}
