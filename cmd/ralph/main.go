// Package main is the entry point for the Ralph CLI application.
//
// Ralph implements the "Ralph Wiggum" technique for iterative AI development
// loops. It continuously feeds prompts to GitHub Copilot, tracking completion
// signals while running tools and edits along the way.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/patbaumgartner/copilot-ralph/internal/cli"
)

var (
	executeCLI           = cli.Execute
	stderr     io.Writer = os.Stderr
)

func main() {
	os.Exit(run())
}

func run() int {
	if err := executeCLI(); err != nil {
		var exitErr *cli.ExitError
		if errors.As(err, &exitErr) {
			if exitErr.Err != nil {
				_, _ = fmt.Fprintln(stderr, "Error:", exitErr.Err)
			}
			return exitErr.Code
		}
		_, _ = fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	return 0
}
