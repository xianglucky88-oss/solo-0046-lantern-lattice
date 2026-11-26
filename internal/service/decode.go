package service

import (
	"errors"
	"net/http"
)

func decodeStatus(err error) int {
	var size *http.MaxBytesError
	if errors.As(err, &size) {
		return 413
	}
	return 400
}
