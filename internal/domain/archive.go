package domain

import (
	"archive/zip"
	"bytes"
)

type zipEntry struct {
	name string
	data []byte
}

func pack(metadata, atlas []byte) ([]byte, error) {
	return packEntries([]zipEntry{{"layout.json", metadata}, {"probes.png", atlas}})
}

// packEntries stores entries uncompressed with zeroed zip timestamps so that
// identical inputs always produce byte-identical archives.
func packEntries(entries []zipEntry) ([]byte, error) {
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, file := range entries {
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

// DownloadName picks a descriptive attachment filename from archive contents.
func DownloadName(blob []byte) string {
	zr, err := zip.NewReader(bytes.NewReader(blob), int64(len(blob)))
	if err != nil {
		return Filename
	}
	if findZipEntry(zr, CoarseEntry) != nil {
		return "coarse-volume.zip"
	}
	if findZipEntry(zr, FineEntry) != nil {
		return "fine-volume.zip"
	}
	return Filename
}
