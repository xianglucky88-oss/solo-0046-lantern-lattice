package service

import "net/http"

func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /v1/example", s.example)
	mux.HandleFunc("POST /v1/build", s.build)
	mux.HandleFunc("POST /v1/import", s.importArchive)
	mux.HandleFunc("GET /v1/artifacts/{digest}", s.download)
	mux.HandleFunc("GET /v1/artifacts/{digest}/sample", s.sampleArtifact)
	return mux
}
