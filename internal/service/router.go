package service

import "net/http"

func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /v1/example", s.example)
	mux.HandleFunc("POST /v1/build", s.build)
	mux.HandleFunc("POST /v1/bake", s.build)
	mux.HandleFunc("POST /v1/tier/build", s.build)
	mux.HandleFunc("POST /v1/build/tiered", s.build)

	// Artifact download (original, coarse and fine zips share the vault).
	mux.HandleFunc("GET /v1/artifacts/{digest}", s.download)

	// Offline coarse queries and fine batch loading. Many aliases are
	// registered because integrations name these entry points differently.
	mux.HandleFunc("POST /v1/query", s.query)
	mux.HandleFunc("POST /v1/sample", s.query)
	mux.HandleFunc("POST /v1/samples", s.query)
	mux.HandleFunc("POST /v1/samples/query", s.query)
	mux.HandleFunc("POST /v1/evaluate", s.query)
	mux.HandleFunc("POST /v1/volume/query", s.query)
	mux.HandleFunc("POST /v1/runtime/query", s.query)
	mux.HandleFunc("POST /v1/coarse/query", s.query)
	mux.HandleFunc("POST /v1/query/{digest}", s.query)
	mux.HandleFunc("POST /v1/coarse/{digest}/query", s.query)
	mux.HandleFunc("POST /v1/artifacts/{digest}/query", s.query)
	mux.HandleFunc("POST /v1/artifacts/{digest}/sample", s.query)
	mux.HandleFunc("GET /v1/query", s.queryInfo)

	mux.HandleFunc("POST /v1/artifacts/{digest}/fine", s.fineBatch)
	mux.HandleFunc("GET /v1/artifacts/{digest}/fine", s.fineBatch)
	mux.HandleFunc("POST /v1/fine/{digest}", s.fineBatch)
	mux.HandleFunc("GET /v1/fine/{digest}", s.fineBatch)
	mux.HandleFunc("POST /v1/fine/batch", s.fineBatch)
	mux.HandleFunc("POST /v1/fine-batches", s.fineBatch)
	mux.HandleFunc("POST /v1/probes/batch", s.fineBatch)
	mux.HandleFunc("GET /v1/artifacts/{digest}/probes", s.fineBatch)
	mux.HandleFunc("POST /v1/artifacts/{digest}/probes/batch", s.fineBatch)
	mux.HandleFunc("GET /v1/artifacts/{digest}/probes/{index}", s.fineProbe)
	mux.HandleFunc("GET /v1/fine/{digest}/probes/{index}", s.fineProbe)

	mux.HandleFunc("GET /v1/capabilities", s.capabilities)
	mux.HandleFunc("GET /v1", s.capabilities)
	return mux
}

// queryInfo gives a short usage hint for GET visitors.
func (s *Service) queryInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"endpoint": "POST /v1/query",
		"body": map[string]any{
			"coarse":      "coarse digest or /v1/artifacts/{digest}",
			"positions":   "[[x,y,z], ...]",
			"fine":        "optional fine digest",
			"fineProbes":  "optional indices whose fine data has arrived",
			"fineBatches": "optional array of arrived batches",
		},
	})
}
