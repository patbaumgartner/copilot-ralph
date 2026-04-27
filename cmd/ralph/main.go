// Package main is the entry point for the Ralph CLI application.
//
// Ralph implements the "Ralph Wiggum" technique for iterative AI development
// loops. It continuously feeds prompts to GitHub Copilot, tracking completion
// signals while running tools and edits along the way.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/patbaumgartner/copilot-ralph/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		var exitErr *cli.ExitError
		if errors.As(err, &exitErr) {
			if exitErr.Err != nil {
				fmt.Fprintln(os.Stderr, "Error:", exitErr.Err)
			}
			os.Exit(exitErr.Code)
		}
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
