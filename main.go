package main

import (
	"fmt"

	petname "github.com/dustinkirkland/golang-petname"
)

func main() {
	for i := 0; i < 10000; i++ {
		name := petname.Generate(3, "-")
		fmt.Printf("│││││││││││││ %30s │││││││││││││││\n", name)
	}
}
