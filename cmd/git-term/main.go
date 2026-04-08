package main

import (
	"fmt"
	"os"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println("git-term", version)
		return
	}
	fmt.Println("git-term: not yet implemented")
}
