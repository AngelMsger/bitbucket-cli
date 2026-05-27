// Command bitbucket-cli lets coding agents and humans drive Bitbucket
// repositories and pull requests from the terminal.
package main

import (
	"os"

	"github.com/angelmsger/bitbucket-cli/internal/app"
)

func main() {
	os.Exit(app.Execute())
}
