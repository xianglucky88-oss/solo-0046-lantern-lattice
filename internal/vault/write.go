package vault

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

func (v Vault) Put(data []byte) (string, bool, error) {
	d := checksum(data)
	if _, err := v.Get(d); !errors.Is(err, fs.ErrNotExist) {
		return d, false, err
	}
	if err := os.MkdirAll(v.Dir, 0700); err != nil {
		return d, false, err
	}
	temp, err := stage(v.Dir, data)
	if err != nil {
		return d, false, err
	}
	defer os.Remove(temp)
	err = os.Link(temp, filepath.Join(v.Dir, d))
	if errors.Is(err, fs.ErrExist) {
		_, err = v.Get(d)
		return d, false, err
	}
	return d, err == nil, err
}
