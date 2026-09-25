// Command api-balance queries and monitors AI API balances/quotas.
package main

import (
	"os"

	"github.com/redtidev1918/api-balance/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}