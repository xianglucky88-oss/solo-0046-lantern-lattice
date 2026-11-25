package service

import (
	"encoding/json"
	"errors"
	"example.local/lantern-lattice/internal/domain"
	"io"
	"mime"
	"net/http"
)

func decode(w http.ResponseWriter, r *http.Request) (domain.Request, int, error) {
	var input domain.Request
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return input, 415, errors.New("expected application/json")
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 3<<20))
	d.DisallowUnknownFields()
	if err := d.Decode(&input); err != nil {
		return input, decodeStatus(err), errors.New("invalid request document")
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return input, decodeStatus(err), errors.New("expected exactly one JSON object")
	}
	return input, 0, nil
}
func decodeStatus(err error) int {
	var size *http.MaxBytesError
	if errors.As(err, &size) {
		return 413
	}
	return 400
}
