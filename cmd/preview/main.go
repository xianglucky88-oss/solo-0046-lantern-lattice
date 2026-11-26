// Command preview recolors reflection faces using only a downloaded package.
//
//	go run ./cmd/preview probe-volume.zip --set wall_a=1,0,0 -o out/
package main

import (
	"os"

	"example.local/lantern-lattice/internal/previewcli"
)

func main() { os.Exit(previewcli.Run(os.Args[1:])) }
