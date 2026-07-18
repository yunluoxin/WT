package main

import (
	"wt/internal/cli"
)

var (
	version = "dev"
	commit  = "none"
)

func main() {
	cli.SetVersion(version, commit)
	cli.Execute()
}
