// Command pocketcontext-atlas is PocketContext Atlas: the server and client in one binary.
package main

import (
	"os"

	"github.com/pocketcontext-labs/pocketcontext-atlas/internal/client"
)

func main() {
	os.Exit(client.Run(os.Args[1:]))
}
