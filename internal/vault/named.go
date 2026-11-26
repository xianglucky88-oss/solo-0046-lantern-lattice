package vault

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// ErrName means a named object identifier is malformed or unsafe.
var ErrName = errors.New("malformed object name")

func safeName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	for _, c := range name {
		if c == '/' || c == '\\' || c == filepath.Separator || c < 0x20 {
			return false
		}
	}
	return true
}

// PutNamed stores data under a fixed name without overwriting anything. It
// returns created=false when an identical name already exists; existing
// versions and patches are never replaced.
func (v Vault) PutNamed(name string, data []byte) (bool, error) {
	if !safeName(name) {
		return false, ErrName
	}
	if err := os.MkdirAll(v.Dir, 0700); err != nil {
		return false, err
	}
	target := filepath.Join(v.Dir, name)
	if _, err := os.Stat(target); err == nil {
		return false, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	temp, err := stage(v.Dir, data)
	if err != nil {
		return false, err
	}
	defer os.Remove(temp)
	if err := os.Link(temp, target); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// ReadNamed reads a previously stored named object.
func (v Vault) ReadNamed(name string) ([]byte, error) {
	if !safeName(name) {
		return nil, ErrName
	}
	return os.ReadFile(filepath.Join(v.Dir, name))
}
