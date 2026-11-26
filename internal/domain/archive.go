package domain

import (
	"archive/zip"
	"bytes"
)

const (
	Filename     = "probe-volume.zip"
	TimelineName = "timeline.json"
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

// packMotion keeps the original two entries uncompressed (Store) so the
// texture contract is unchanged, while the time-varying timeline uses
// Deflate because it holds the full piecewise-constant probe data.
func packMotion(metadata, atlas, timeline []byte) ([]byte, error) {
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, file := range []struct {
		name   string
		data   []byte
		method uint16
	}{
		{"layout.json", metadata, zip.Store},
		{"probes.png", atlas, zip.Store},
		{TimelineName, timeline, zip.Deflate},
	} {
		dest, err := zw.CreateHeader(&zip.FileHeader{Name: file.name, Method: file.method})
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
