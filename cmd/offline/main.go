// Command offline reconstructs a continued-baking artifact without any server.
// It needs only the supplement delivered for the continuation, the previous
// build receipt, and the previous artifact ZIP. Both the previous light
// version identity and the previous ZIP bytes must match or it refuses.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"example.local/lantern-lattice/internal/domain"
	"flag"
	"fmt"
	"io"
	"os"
)

func main() {
	patchPath := flag.String("patch", "", "path to the continuation supplement JSON")
	receiptPath := flag.String("receipt", "", "path to the previous build receipt JSON")
	zipPath := flag.String("zip", "", "path to the previous artifact ZIP")
	outPath := flag.String("out", domain.Filename, "path for the reconstructed ZIP")
	flag.Parse()
	if *patchPath == "" || *receiptPath == "" || *zipPath == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: offline -patch supplement.json -receipt receipt.json -zip previous.zip [-out probe-volume.zip]")
		os.Exit(2)
	}
	if err := run(*patchPath, *receiptPath, *zipPath, *outPath); err != nil {
		fmt.Fprintln(os.Stderr, "offline apply refused:", err)
		os.Exit(1)
	}
}

func run(patchPath, receiptPath, zipPath, outPath string) error {
	patchData, err := os.ReadFile(patchPath)
	if err != nil {
		return fmt.Errorf("read patch: %w", err)
	}
	patch, err := domain.DecodePatch(patchData)
	if err != nil {
		return err
	}
	receiptData, err := os.ReadFile(receiptPath)
	if err != nil {
		return fmt.Errorf("read receipt: %w", err)
	}
	var receipt domain.PatchReceipt
	if err := decodeStrict(receiptData, &receipt); err != nil {
		return errors.New("invalid receipt document")
	}
	if receipt.Digest == "" || receipt.Version == "" {
		return errors.New("receipt lacks version identity or digest")
	}
	previous, err := os.ReadFile(zipPath)
	if err != nil {
		return fmt.Errorf("read previous artifact: %w", err)
	}
	out, err := domain.ApplyPatch(patch, receipt, previous)
	if err != nil {
		return err
	}
	return os.WriteFile(outPath, out, 0600)
}

func decodeStrict(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("extra JSON content")
	}
	return nil
}
