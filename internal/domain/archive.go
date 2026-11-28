package domain

import (
	"archive/zip"
	"bytes"
	"fmt"
)

func textureName(i int) string { return fmt.Sprintf("probes-%d.png", i) }

// pack writes layout.json plus one six-face atlas per grid. Each grid keeps its
// own name so both textures download from the same artifact.
func pack(metadata []byte, names []string, textures [][]byte) ([]byte, error) {
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	files := []struct {
		name string
		data []byte
	}{{"layout.json", metadata}}
	for i, data := range textures {
		files = append(files, struct {
			name string
			data []byte
		}{names[i], data})
	}
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
