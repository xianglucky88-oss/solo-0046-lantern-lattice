// Command preview relights a downloaded probe-volume.zip using only the
// artifact's embedded linear basis. It is the native counterpart of
// tools/preview.py and accepts the same common flag spellings.
//
// Usage:
//
//	preview artifact.zip -sky 0.25 -lights 1 -at 0.4,0,0 [-out dir]
package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"example.local/lantern-lattice/internal/domain"
)

type multiCoord []domain.Vec

func (m *multiCoord) String() string { return "" }
func (m *multiCoord) Set(v string) error {
	parts := strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ' ' || r == ';' })
	if len(parts) != 3 {
		return fmt.Errorf("coordinate must be x,y,z")
	}
	var p domain.Vec
	for i, x := range parts {
		f, err := strconv.ParseFloat(x, 64)
		if err != nil {
			return err
		}
		p[i] = f
	}
	*m = append(*m, p)
	return nil
}

func main() {
	sky := flag.Float64("sky", 1, "sky multiplier [0..1]")
	lights := flag.Float64("lights", 1, "point-light multiplier [0..1]")
	flag.Float64Var(sky, "sky-scale", 1, "alias of -sky")
	flag.Float64Var(sky, "sky-multiplier", 1, "alias of -sky")
	flag.Float64Var(lights, "light", 1, "alias of -lights")
	flag.Float64Var(lights, "light-scale", 1, "alias of -lights")
	flag.Float64Var(lights, "lights-scale", 1, "alias of -lights")
	flag.Float64Var(lights, "lamp-scale", 1, "alias of -lights")
	outDir := flag.String("out", "", "optional output directory")
	jsonOut := flag.String("json", "", "write JSON report to file")
	pngOut := flag.String("png", "", "write relit PNG to file")
	var at multiCoord
	flag.Var(&at, "at", "position x,y,z (repeatable)")
	flag.Var(&at, "position", "alias of -at")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: preview <artifact.zip> [-sky f] [-lights f] [-at x,y,z]")
		os.Exit(2)
	}
	raw, err := os.ReadFile(args[0])
	if err != nil {
		fatal(err)
	}
	res, err := domain.Preview(raw, *sky, *lights, at)
	if err != nil {
		fatal(err)
	}
	b64 := base64.StdEncoding.EncodeToString(res.PNG)
	report := map[string]any{
		"ok": true, "Schema": domain.PreviewSchema,
		"Exposure": res.Artifact.Exposure,
		"Width":    res.Width, "Height": res.Height,
		"SkyScale": res.SkyScale, "LightScale": res.LightScale,
		"Sky": res.SkyScale, "Lights": res.LightScale,
		"PNG": b64, "png_base64": b64, "Samples": res.Samples,
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		fatal(err)
	}
	if *outDir != "" {
		if err := os.MkdirAll(*outDir, 0o755); err != nil {
			fatal(err)
		}
		_ = os.WriteFile(*outDir+"/probes.png", res.PNG, 0o644)
		_ = os.WriteFile(*outDir+"/samples.json", encoded, 0o644)
	}
	if *pngOut != "" {
		_ = os.WriteFile(*pngOut, res.PNG, 0o644)
	}
	if *jsonOut != "" {
		_ = os.WriteFile(*jsonOut, encoded, 0o644)
	}
	fmt.Println(string(encoded))
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "preview:", err)
	os.Exit(1)
}
