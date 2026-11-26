package service

import (
	"errors"
	"net/http"

	"example.local/lantern-lattice/internal/domain"
)

type Receipt struct {
	Digest   string `json:"digest"`
	Bytes    int    `json:"bytes"`
	Artifact string `json:"artifact"`
}

func (s *Service) build(w http.ResponseWriter, r *http.Request) {
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		w.Header().Set("Retry-After", "1")
		problem(w, 503, "compiler busy")
		return
	}

	input, status, err := decodeBuild(w, r)
	if err != nil {
		problem(w, status, err.Error())
		return
	}
	tierOpts, tierRequested, partial := resolveTierOptions(withQueryAliases(input.raw, r))

	bundle, buildErr := domain.BuildTiered(input.Request, tierOpts)
	var budgetErr *domain.CoarseBudgetError
	coarseFailed := buildErr != nil && errors.As(buildErr, &budgetErr)

	if buildErr != nil && !(tierRequested && partial && coarseFailed && bundle != nil) {
		if coarseFailed {
			coarseProblem(w, budgetErr)
			return
		}
		problem(w, http.StatusUnprocessableEntity, buildErr.Error())
		return
	}
	if r.Context().Err() != nil {
		return
	}

	digest, created, err := s.vault.Put(bundle.Original)
	if err != nil {
		problem(w, 500, "cannot save artifact")
		return
	}
	location := "/v1/artifacts/" + digest
	w.Header().Set("Location", location)
	statusCode := http.StatusOK
	if created {
		statusCode = http.StatusCreated
	}

	if !tierRequested {
		writeJSON(w, statusCode, Receipt{digest, len(bundle.Original), location})
		return
	}

	var body map[string]any
	if coarseFailed {
		body, err = s.partialReceipt(digest, bundle, budgetErr)
	} else {
		body, err = s.tieredReceipt(digest, bundle)
	}
	if err != nil {
		problem(w, 500, "cannot save tier artifact")
		return
	}
	writeJSON(w, statusCode, body)
}
