// Package previewcli implements the offline recolor command shared by the
// cmd/preview and cmd/recolor entry points.
package previewcli

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"example.local/lantern-lattice/internal/domain"
)

type setFlag map[string]domain.Vec

func (s setFlag) String() string { return "" }
func (s setFlag) Set(v string) error {
	id, color, ok := strings.Cut(v, "=")
	if !ok {
		id, color, ok = strings.Cut(v, ":")
		if !ok {
			return fmt.Errorf("expected ID=R,G,B")
		}
	}
	parts := strings.Split(color, ",")
	if len(parts) != 3 {
		return fmt.Errorf("color needs 3 comma separated channels")
	}
	var vec domain.Vec
	for i, p := range parts {
		if _, err := fmt.Sscanf(strings.TrimSpace(p), "%g", &vec[i]); err != nil {
			return err
		}
	}
	s[strings.TrimSpace(id)] = vec
	return nil
}

// Run executes the CLI and returns the process exit code.
func Run(argv []string) int {
	overrides := setFlag{}
	fs := flag.NewFlagSet("preview", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Var(overrides, "set", "face ID=R,G,B (repeatable)")
	changes := fs.String("changes", "", "JSON file mapping face IDs to [r,g,b]")
	outDir := fs.String("out", "", "directory for png/json/layout/zip outputs")
	pngPath := fs.String("png", "", "explicit PNG output path")
	jsonPath := fs.String("json", "", "explicit linear samples JSON path")
	zipPath := fs.String("zip-out", "", "recolored package zip output path")
	quiet := fs.Bool("quiet", false, "suppress stdout report")
	fs.BoolVar(quiet, "q", false, "suppress stdout report")
	help := fs.Bool("help", false, "show help")
	fs.BoolVar(help, "h", false, "show help")

	args := reorderFlags(argv, map[string]bool{
		"set": true, "changes": true, "out": true,
		"png": true, "json": true, "zip-out": true,
	})
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *help {
		fmt.Println("usage: preview PACKAGE.zip [--set ID=R,G,B] [--changes f.json] [-out dir]")
		return 0
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprintln(os.Stderr, "usage: preview PACKAGE.zip [--set ID=R,G,B] [--changes f.json] [-out dir]")
		return 2
	}
	pkgBytes, err := os.ReadFile(rest[0])
	if err != nil {
		return fail(err)
	}
	if *changes != "" {
		doc, err := os.ReadFile(*changes)
		if err != nil {
			return fail(err)
		}
		var m map[string]domain.Vec
		if err := json.Unmarshal(doc, &m); err != nil {
			return fail(err)
		}
		for id, v := range m {
			overrides[id] = v
		}
	}

	result, err := domain.RecolorZip(pkgBytes, map[string]domain.Vec(overrides))
	if err != nil {
		return fail(err)
	}
	report := map[string]any{
		"Schema":      "lantern-preview-result/v1",
		"Exposure":    result.Layout.Exposure,
		"Reflectance": result.Reflectance,
		"Samples":     result.Samples,
		"Lobes":       result.Lobes,
		"png_base64":  base64.StdEncoding.EncodeToString(result.PNG),
	}
	if *outDir != "" {
		if err := os.MkdirAll(*outDir, 0o755); err != nil {
			return fail(err)
		}
		if err := os.WriteFile(filepath.Join(*outDir, "probes.png"), result.PNG, 0o644); err != nil {
			return fail(err)
		}
		if err := os.WriteFile(filepath.Join(*outDir, "layout.json"), result.LayoutJSON, 0o644); err != nil {
			return fail(err)
		}
		if err := os.WriteFile(filepath.Join(*outDir, "probe-volume-recolored.zip"), result.ZIP, 0o644); err != nil {
			return fail(err)
		}
		if *jsonPath == "" {
			p := filepath.Join(*outDir, "samples.json")
			jsonPath = &p
		}
	}
	if *pngPath != "" {
		if err := os.WriteFile(*pngPath, result.PNG, 0o644); err != nil {
			return fail(err)
		}
	}
	if *zipPath != "" {
		if err := os.WriteFile(*zipPath, result.ZIP, 0o644); err != nil {
			return fail(err)
		}
	}
	if *jsonPath != "" {
		raw, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return fail(err)
		}
		if err := os.WriteFile(*jsonPath, raw, 0o644); err != nil {
			return fail(err)
		}
	}
	if !*quiet {
		raw, err := json.Marshal(report)
		if err != nil {
			return fail(err)
		}
		fmt.Println(string(raw))
	}
	return 0
}

func fail(err error) int {
	fmt.Fprintln(os.Stderr, "preview:", err)
	return 1
}

// reorderFlags moves all options (and their values) ahead of positional
// arguments so Go's flag parser accepts "pkg.zip --set a=1,0,0" as well.
func reorderFlags(argv []string, valueFlags map[string]bool) []string {
	var flags, positional []string
	for i := 0; i < len(argv); i++ {
		tok := argv[i]
		switch {
		case tok == "--":
			positional = append(positional, argv[i+1:]...)
			i = len(argv)
		case strings.HasPrefix(tok, "-") && tok != "-":
			rawName := strings.SplitN(tok, "=", 2)[0]
			name := strings.TrimLeft(rawName, "-")
			flags = append(flags, tok)
			if valueFlags[name] && !strings.Contains(tok, "=") && i+1 < len(argv) {
				i++
				flags = append(flags, argv[i])
			}
		default:
			positional = append(positional, tok)
		}
	}
	return append(flags, positional...)
}
