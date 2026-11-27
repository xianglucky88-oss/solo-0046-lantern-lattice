package domain

import (
	"archive/zip"
	"bytes"
	"io"
)

func pack(metadata, atlas []byte) ([]byte, error) {
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, file := range []struct {
		name string
		data []byte
	}{{"layout.json", metadata}, {"probes.png", atlas}} {
		dest, err := zw.CreateHeader(&zip.FileHeader{Name: file.name, Method: zip.Store})
		if err != nil {
			return nil, err
		}
		if _, err = dest.Write(file.data); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// ReadArtifactFiles extracts layout.json and probes.png from a product zip.
func ReadArtifactFiles(zipBytes []byte) (layoutJSON, pngBytes []byte, err error) {
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return nil, nil, err
	}
	for _, f := range zr.File {
		if f.Name != "layout.json" && f.Name != "probes.png" {
			continue
		}
		rc, oerr := f.Open()
		if oerr != nil {
			return nil, nil, oerr
		}
		data, rerr := io.ReadAll(rc)
		_ = rc.Close()
		if rerr != nil {
			return nil, nil, rerr
		}
		switch f.Name {
		case "layout.json":
			layoutJSON = data
		case "probes.png":
			pngBytes = data
		}
	}
	if layoutJSON == nil {
		return nil, nil, io.ErrUnexpectedEOF
	}
	return layoutJSON, pngBytes, nil
}
