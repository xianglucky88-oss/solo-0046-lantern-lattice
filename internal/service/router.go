package service

import "net/http"

func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /v1/example", s.example)
	mux.HandleFunc("POST /v1/build", s.build)
	mux.HandleFunc("GET /v1/artifacts/{digest}", s.download)

	// Artifact-scoped preview / relight.
	mux.HandleFunc("GET /v1/artifacts/{digest}/preview", s.handlePreview(true))
	mux.HandleFunc("POST /v1/artifacts/{digest}/preview", s.handlePreview(false))
	mux.HandleFunc("GET /v1/artifacts/{digest}/relight", s.handlePreview(true))
	mux.HandleFunc("POST /v1/artifacts/{digest}/relight", s.handlePreview(false))
	mux.HandleFunc("GET /v1/artifacts/{digest}/texture", s.handlePreview(true))
	mux.HandleFunc("POST /v1/artifacts/{digest}/texture", s.handlePreview(false))
	for _, p := range []string{"preview.png", "probes.png", "texture.png", "atlas.png", "relight.png", "lit.png"} {
		mux.HandleFunc("GET /v1/artifacts/{digest}/"+p, s.handlePreview(true))
	}

	// Artifact-scoped ad-hoc linear sampling.
	mux.HandleFunc("GET /v1/artifacts/{digest}/sample", s.handleSample)
	mux.HandleFunc("POST /v1/artifacts/{digest}/sample", s.handleSample)
	mux.HandleFunc("GET /v1/artifacts/{digest}/samples", s.handleSample)
	mux.HandleFunc("POST /v1/artifacts/{digest}/samples", s.handleSample)
	mux.HandleFunc("GET /v1/artifacts/{digest}/linear", s.handleSample)
	mux.HandleFunc("POST /v1/artifacts/{digest}/linear", s.handleSample)
	mux.HandleFunc("POST /v1/artifacts/{digest}/query", s.handleSample)

	// Further vocabulary aliases for the same preview/sample operation.
	for _, p := range []string{"lighting", "lookdev", "lights", "dimmer", "adjust"} {
		mux.HandleFunc("GET /v1/artifacts/{digest}/"+p, s.handlePreview(true))
		mux.HandleFunc("POST /v1/artifacts/{digest}/"+p, s.handlePreview(false))
	}

	// Digest/archive supplied in the body (or as an uploaded zip).
	mux.HandleFunc("POST /v1/preview", s.handlePreview(false))
	mux.HandleFunc("POST /v1/relight", s.handlePreview(false))
	mux.HandleFunc("GET /v1/preview/{digest}", s.handlePreview(true))
	mux.HandleFunc("POST /v1/preview/{digest}", s.handlePreview(false))
	mux.HandleFunc("GET /v1/relight/{digest}", s.handlePreview(true))
	mux.HandleFunc("POST /v1/relight/{digest}", s.handlePreview(false))
	mux.HandleFunc("POST /v1/sample", s.handleSample)
	mux.HandleFunc("POST /v1/samples", s.handleSample)
	mux.HandleFunc("POST /v1/query", s.handleSample)
	mux.HandleFunc("POST /v1/lookdev", s.handlePreview(false))
	mux.HandleFunc("POST /v1/adjust", s.handlePreview(false))
	mux.HandleFunc("POST /v1/lighting", s.handlePreview(false))
	mux.HandleFunc("POST /v1/dimmer", s.handlePreview(false))
	return mux
}
