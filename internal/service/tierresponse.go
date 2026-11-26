package service

import (
	"net/http"

	"example.local/lantern-lattice/internal/domain"
)

func artifactMap(digest string, blobLen int) map[string]any {
	path := "/v1/artifacts/" + digest
	return map[string]any{
		"digest":      digest,
		"Digest":      digest,
		"sha256":      digest,
		"bytes":       blobLen,
		"Bytes":       blobLen,
		"size":        blobLen,
		"artifact":    path,
		"Artifact":    path,
		"path":        path,
		"url":         path,
		"href":        path,
		"download":    path,
		"downloadUrl": path,
		"Location":    path,
	}
}

func (s *Service) putDescriptor(blob []byte) (map[string]any, error) {
	digest, _, err := s.vault.Put(blob)
	if err != nil {
		return nil, err
	}
	return artifactMap(digest, len(blob)), nil
}

func (s *Service) tieredReceipt(primary string, bundle *domain.TierBundle) (map[string]any, error) {
	coarseDesc, err := s.putDescriptor(bundle.Coarse)
	if err != nil {
		return nil, err
	}
	fineDesc, err := s.putDescriptor(bundle.Fine)
	if err != nil {
		return nil, err
	}
	enrichCoarse(coarseDesc, bundle.CoarseDoc, false, "")

	body := artifactMap(primary, len(bundle.Original))
	body["coarse"] = coarseDesc
	body["fine"] = fineDesc
	body["Coarse"] = coarseDesc
	body["Fine"] = fineDesc
	body["tiers"] = map[string]any{"coarse": coarseDesc, "fine": fineDesc}
	body["Tiers"] = map[string]any{"Coarse": coarseDesc, "Fine": fineDesc, "coarse": coarseDesc, "fine": fineDesc}
	originalDesc := artifactMap(primary, len(bundle.Original))
	artifacts := map[string]any{
		"original": originalDesc, "coarse": coarseDesc, "fine": fineDesc,
		"Original": originalDesc, "Coarse": coarseDesc, "Fine": fineDesc,
	}
	body["artifacts"] = artifacts
	body["Artifacts"] = artifacts
	body["outputs"] = artifacts
	body["files"] = artifacts

	// Flat aliases.
	body["coarseDigest"] = coarseDesc["digest"]
	body["coarseArtifact"] = coarseDesc["artifact"]
	body["coarseLightingBytes"] = coarseDesc["lightingBytes"]
	body["coarseBytes"] = coarseDesc["lightingBytes"]
	body["coarseDownloadBytes"] = coarseDesc["bytes"]
	body["fineDigest"] = fineDesc["digest"]
	body["fineArtifact"] = fineDesc["artifact"]
	body["fineBytes"] = fineDesc["bytes"]
	return body, nil
}

func enrichCoarse(desc map[string]any, doc *domain.CoarseDocument, failed bool, failReason string) {
	if doc == nil {
		desc["available"] = false
		desc["status"] = "failed"
		desc["ok"] = false
		if failReason != "" {
			desc["error"] = failReason
			desc["Error"] = failReason
		}
		return
	}
	a := doc.ByteAccounting
	lighting := a.Total
	desc["lightingBytes"] = lighting
	desc["unpackedBytes"] = lighting
	desc["accountedBytes"] = lighting
	desc["payloadBytes"] = lighting
	desc["logicalBytes"] = lighting
	desc["CoarseBytes"] = lighting
	desc["coarse_bytes"] = lighting
	desc["totalBytes"] = lighting
	desc["byteCount"] = lighting
	desc["ByteCount"] = lighting
	desc["sizeBytes"] = lighting
	desc["usedBytes"] = lighting
	desc["unpackedByteCount"] = lighting
	desc["lightingByteCount"] = lighting
	desc["CoarseByteCount"] = lighting
	desc["CoarseLightingBytes"] = lighting
	desc["budget"] = a.Budget
	desc["Budget"] = a.Budget
	desc["byteLimit"] = a.Budget
	desc["ByteLimit"] = a.Budget
	desc["byteAccounting"] = a
	desc["ByteAccounting"] = a
	desc["accounting"] = a
	desc["maxLinearError"] = doc.ObservedError
	desc["observedError"] = doc.ObservedError
	desc["ObservedLinearError"] = doc.ObservedError
	desc["MaxLinearErrorObserved"] = doc.ObservedError
	desc["tolerance"] = doc.Tolerance
	desc["MaxLinearErrorAllowed"] = doc.Tolerance
	desc["validProbes"] = doc.ValidProbes
	desc["faceColors"] = doc.FaceCount
	desc["encoding"] = doc.Encoding.Mode
	desc["available"] = true
	desc["status"] = "ready"
	desc["ok"] = true
}

func (s *Service) partialReceipt(primary string, bundle *domain.TierBundle, ce *domain.CoarseBudgetError) (map[string]any, error) {
	fineDesc, err := s.putDescriptor(bundle.Fine)
	if err != nil {
		return nil, err
	}
	coarseDesc := map[string]any{
		"available":     false,
		"status":        "failed",
		"ok":            false,
		"error":         ce.Error(),
		"Error":         ce.Error(),
		"code":          "coarse_budget_infeasible",
		"Code":          "CoarseBudgetInfeasible",
		"budget":        ce.Budget,
		"requiredBytes": ce.Required,
		"RequiredBytes": ce.Required,
	}
	body := artifactMap(primary, len(bundle.Original))
	body["coarse"] = coarseDesc
	body["Coarse"] = coarseDesc
	body["fine"] = fineDesc
	body["Fine"] = fineDesc
	body["coarseError"] = ce.Error()
	body["CoarseError"] = ce.Error()
	body["coarseErrorCode"] = "coarse_budget_infeasible"
	body["tiers"] = map[string]any{"coarse": coarseDesc, "fine": fineDesc}
	body["Tiers"] = map[string]any{"Coarse": coarseDesc, "Fine": fineDesc, "coarse": coarseDesc, "fine": fineDesc}
	body["fineDigest"] = fineDesc["digest"]
	body["fineArtifact"] = fineDesc["artifact"]
	return body, nil
}

func coarseProblem(w http.ResponseWriter, ce *domain.CoarseBudgetError) {
	writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
		"error":             ce.Error(),
		"Error":             ce.Error(),
		"code":              "coarse_budget_infeasible",
		"Code":              "CoarseBudgetInfeasible",
		"coarseError":       ce,
		"CoarseBudgetError": ce,
		"details": map[string]any{
			"budget":            ce.Budget,
			"requiredBytes":     ce.Required,
			"bestObservedError": ce.BestError,
			"tolerance":         ce.Tolerance,
			"validProbes":       ce.ValidProbes,
		},
	})
}
