package main

import (
	"os"

	"github.com/apandurangi/cli/commands"
)

func main() {
	//Initialize the commands
	os.Setenv("GO111MODULE", "on")
	commands.Initialize()
	commands.Execute()
}
