// Command recolor is an alias for cmd/preview.
//
//	go run ./cmd/recolor probe-volume.zip --set wall_a=1,0,0 -o out/
package main

import (
	"os"

	"example.local/lantern-lattice/internal/previewcli"
)

func main() { os.Exit(previewcli.Run(os.Args[1:])) }
