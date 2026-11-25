package service

import (
	_ "embed"
	"net/http"
)

//go:embed demo.json
var demo []byte

func (s *Service) example(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(demo)
}
