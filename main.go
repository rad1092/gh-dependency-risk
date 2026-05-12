package main

import (
	"os"

	"github.com/rad1092/gh-dependency-risk/cmd"
)

func main() {
	os.Exit(cmd.Execute(os.Args[1:]))
}
