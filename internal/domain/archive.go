package domain

import (
	"archive/zip"
	"bytes"
)

type archiveFile struct {
	name string
	data []byte
}

func writeZip(files []archiveFile) ([]byte, error) {
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, file := range files {
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

func pack(metadata, atlas []byte) ([]byte, error) {
	return writeZip([]archiveFile{
		{name: "layout.json", data: metadata},
		{name: "probes.png", data: atlas},
	})
}

func packTiled(metadata []byte, pages []renderedPage) ([]byte, error) {
	files := make([]archiveFile, 0, len(pages)+1)
	files = append(files, archiveFile{name: "layout.json", data: metadata})
	for _, pg := range pages {
		data, err := encodePNG(pg.image)
		if err != nil {
			return nil, err
		}
		files = append(files, archiveFile{name: pg.spec.File, data: data})
	}
	return writeZip(files)
}
