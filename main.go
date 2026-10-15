package main

import (
	"os"

	"github.com/wkt84/esapi-cli/cmd"
)

func main() {
	os.Exit(cmd.Run(os.Args[1:], os.Stdout, os.Stderr))
}
