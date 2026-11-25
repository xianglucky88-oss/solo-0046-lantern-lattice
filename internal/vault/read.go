package vault

import (
	"errors"
	"os"
	"path/filepath"
)

var ErrDigest = errors.New("malformed digest")
var ErrIntegrity = errors.New("artifact does not match digest")

type Vault struct{ Dir string }

func (v Vault) Get(digest string) ([]byte, error) {
	if !validDigest(digest) {
		return nil, ErrDigest
	}
	data, err := os.ReadFile(filepath.Join(v.Dir, digest))
	if err != nil {
		return nil, err
	}
	if checksum(data) != digest {
		return nil, ErrIntegrity
	}
	return data, nil
}
