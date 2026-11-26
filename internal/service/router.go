package service

import "net/http"

func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /v1/example", s.example)
	mux.HandleFunc("POST /v1/build", s.build)
	mux.HandleFunc("POST /v1/resources", s.bakeResource)
	mux.HandleFunc("POST /v1/resources/bake", s.bakeResource)
	mux.HandleFunc("POST /v1/probe-resources", s.bakeResource)
	mux.HandleFunc("POST /v1/instances/sample", s.sampleInstance)
	mux.HandleFunc("POST /v1/instance/sample", s.sampleInstance)
	mux.HandleFunc("POST /v1/probes/sample", s.sampleInstance)
	mux.HandleFunc("POST /v1/resources/sample", s.sampleInstance)
	mux.HandleFunc("POST /v1/instance-samples", s.sampleInstance)
	mux.HandleFunc("GET /v1/artifacts/{digest}", s.download)
	return mux
}
