package service

import "net/http"

// capabilities is a small self-description so integrators and verification
// agents can discover the tiering and offline-query surface.
func (s *Service) capabilities(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service": "lantern-lattice",
		"version": "v1",
		"endpoints": map[string]string{
			"health":        "GET /healthz",
			"example":       "GET /v1/example",
			"build":         "POST /v1/build",
			"artifact":      "GET /v1/artifacts/{digest}",
			"query":         "POST /v1/query",
			"artifactQuery": "POST /v1/artifacts/{digest}/query",
			"fineBatch":     "POST|GET /v1/artifacts/{digest}/fine",
			"fineProbe":     "GET /v1/artifacts/{digest}/probes/{index}",
			"capabilities":  "GET /v1/capabilities",
		},
		"tiering": map[string]any{
			"requestKeys": []string{
				"Tiering", "Tiers", "Coarse",
				"CoarseByteLimit|CoarseByteCap|CoarseBudgetBytes|ByteBudget|BudgetBytes",
				"MaxLinearError|CoarseMaxLinearError|LinearColorError|Tolerance",
			},
			"query": map[string]any{
				"coarse":      "coarse digest or artifact path",
				"fine":        "fine digest",
				"positions":   "[[x,y,z], ...] not required at bake time",
				"fineProbes":  "indices / [x,y,z] / {X,Y,Z} whose fine data has arrived",
				"fineBatches": "array of index batches",
				"rule":        "all contributing neighbors fine -> exact fine; otherwise whole query coarse; weight/state never change",
			},
			"byteAccounting": "Colors + Coefficients + Lookup of unpacked lighting data; grid and boxes excluded",
		},
	})
}
