// Command readprobe answers one receiver-normal lighting query from a
// downloaded artifact, without contacting the server. It needs only the
// artifact ZIP (or its layout.json) plus a position and a normal.
//
//	readprobe -artifact probe-volume.zip -point 0.5,0.5,0.5 -normal 0,0,1
//
// The artifact must have been built with receiver normals; otherwise the
// command refuses, matching the HTTP readout.
package main

import (
	"encoding/json"
	"errors"
	"example.local/lantern-lattice/internal/domain"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func main() {
	artifact := flag.String("artifact", "", "download artifact ZIP or layout.json")
	point := flag.String("point", "", "receiver position x,y,z")
	normal := flag.String("normal", "", "receiver normal x,y,z")
	flag.Parse()
	if *artifact == "" || *point == "" || *normal == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: readprobe -artifact probe-volume.zip -point x,y,z -normal x,y,z")
		os.Exit(2)
	}
	position, err := parseVec(*point)
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid point:", err)
		os.Exit(2)
	}
	direction, err := parseVec(*normal)
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid normal:", err)
		os.Exit(2)
	}
	data, err := os.ReadFile(*artifact)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read artifact:", err)
		os.Exit(1)
	}
	resource, err := domain.ReadNormalResource(data)
	if err != nil {
		fmt.Fprintln(os.Stderr, "offline readout refused:", err)
		os.Exit(1)
	}
	result := resource.Sample(position, direction)
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		fmt.Fprintln(os.Stderr, "write result:", err)
		os.Exit(1)
	}
}

func parseVec(text string) (domain.Vec, error) {
	parts := strings.Split(text, ",")
	if len(parts) != 3 {
		return domain.Vec{}, errors.New("expected three comma separated components")
	}
	var out domain.Vec
	for i, p := range parts {
		v, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			return domain.Vec{}, err
		}
		out[i] = v
	}
	return out, nil
}
