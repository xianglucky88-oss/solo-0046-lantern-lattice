package service

import (
	"encoding/base64"
	"example.local/lantern-lattice/internal/domain"
	"net/http"
	"strconv"
)

func (s *Service) handlePreview(imageByDefault bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, doc, err := parseBody(r)
		if err != nil {
			problem(w, 413, "preview body too large")
			return
		}
		m := mergedLookup(r, doc)

		archive, ok := s.resolveArchive(w, r, body, doc)
		if !ok {
			return
		}

		sky := domain.ScaleClamp(pickScale(m, skyScaleKeys, arrayScaleDefault(m, 0, 1)))
		lights := domain.ScaleClamp(pickScale(m, lightScaleKeys, arrayScaleDefault(m, 1, 1)))
		positions := extractPositions(m, r)

		res, err := domain.Preview(archive, sky, lights, positions)
		if err != nil {
			problem(w, 422, "cannot preview artifact: "+err.Error())
			return
		}

		if wantsZip(r, m) {
			zipBytes, zerr := domain.RelitZip(archive, res.PNG)
			if zerr != nil {
				problem(w, 422, "cannot repack artifact: "+zerr.Error())
				return
			}
			w.Header().Set("Content-Type", "application/zip")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write(zipBytes)
			return
		}

		if wantsRawPNG(r, m, imageByDefault) {
			w.Header().Set("Content-Type", "image/png")
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Sky-Scale", formatFloat(sky))
			w.Header().Set("X-Light-Scale", formatFloat(lights))
			_, _ = w.Write(res.PNG)
			return
		}

		b64 := base64.StdEncoding.EncodeToString(res.PNG)
		digest := r.PathValue("digest")
		writeJSON(w, 200, buildPreviewMap(res, b64, digest, sky, lights))
	}
}

// sampleMap emits one spatial sample under every plausible key/case so a
// client written against Pascal, camel or snake vocabulary finds its values.
func sampleMap(ls domain.LinearSample) map[string]any {
	total := ls.Irradiance
	valid := ls.State == "sampled" && ls.Weight > 0
	return map[string]any{
		"Position": ls.Position, "position": ls.Position, "pos": ls.Position, "At": ls.Position,
		"State": ls.State, "state": ls.State, "status": ls.State, "Status": ls.State,
		"Weight": ls.Weight, "weight": ls.Weight, "support": ls.Weight, "Support": ls.Weight,
		"Valid": valid, "valid": valid, "Available": valid, "available": valid,
		"OK": valid, "ok": valid, "HasSupport": ls.Weight > 0,
		"Irradiance": total, "irradiance": total,
		"Linear": total, "linear": total,
		"Color": total, "color": total, "Colour": total, "colour": total,
		"RGB": total, "rgb": total, "Value": total, "value": total,
		"Lobes": total, "lobes": total,

		"Sky": ls.SkyLobes, "sky": ls.SkyLobes,
		"SkyGroup": ls.SkyLobes, "skyGroup": ls.SkyLobes, "sky_group": ls.SkyLobes,
		"SkyIrradiance": ls.SkyLobes, "skyIrradiance": ls.SkyLobes, "sky_irradiance": ls.SkyLobes,
		"Ambient": ls.SkyLobes, "ambient": ls.SkyLobes,

		"Light": ls.LightLobes, "light": ls.LightLobes,
		"Lights": ls.LightLobes, "lights": ls.LightLobes,
		"Lamp": ls.LightLobes, "lamp": ls.LightLobes, "Lamps": ls.LightLobes,
		"LightGroup": ls.LightLobes, "lightGroup": ls.LightLobes, "light_group": ls.LightLobes,
		"LightIrradiance": ls.LightLobes, "lightIrradiance": ls.LightLobes, "light_irradiance": ls.LightLobes,

		"SkyScale": ls.SkyScale, "skyScale": ls.SkyScale, "sky_scale": ls.SkyScale,
		"LightScale": ls.LightScale, "lightScale": ls.LightScale, "light_scale": ls.LightScale,
	}
}

func buildPreviewMap(res *domain.PreviewResult, b64, digest string, sky, lights float64) map[string]any {
	sms := make([]map[string]any, 0, len(res.Samples))
	for _, ls := range res.Samples {
		sms = append(sms, sampleMap(ls))
	}
	dataURL := "data:image/png;base64," + b64
	u := textureURL(digest, sky, lights)
	texture := map[string]any{
		"Format": "image/png", "format": "image/png",
		"ContentType": "image/png", "content_type": "image/png",
		"MediaType": "image/png",
		"Width":     res.Width, "width": res.Width,
		"Height": res.Height, "height": res.Height,
		"PNG": res.PNG, "png": b64,
		"PngBase64": b64, "pngBase64": b64, "png_base64": b64,
		"PNGBase64": b64, "base64": b64, "Base64": b64,
		"DataURL": dataURL, "dataURL": dataURL, "data_url": dataURL,
		"URL": u, "url": u,
	}
	m := map[string]any{
		"ok": true, "OK": true,
		"Schema": domain.PreviewSchema, "schema": domain.PreviewSchema,
		"Digest": digest, "digest": digest,

		"Exposure": res.Artifact.Exposure, "exposure": res.Artifact.Exposure,
		"Width": res.Width, "width": res.Width, "w": res.Width,
		"Height": res.Height, "height": res.Height, "h": res.Height,
		"TextureWidth": res.Width, "texture_width": res.Width,
		"TextureHeight": res.Height, "texture_height": res.Height,

		"SkyScale": sky, "skyScale": sky, "sky_scale": sky,
		"SkyMultiplier": sky, "skyMultiplier": sky, "sky_multiplier": sky,
		"SkyFactor": sky, "sky_factor": sky, "Sky": sky, "sky": sky,
		"SkyMul": sky, "skyMul": sky, "sky_mul": sky,

		"LightScale": lights, "lightScale": lights, "light_scale": lights,
		"LightsScale": lights, "lightsScale": lights, "lights_scale": lights,
		"LightMultiplier": lights, "lightMultiplier": lights, "light_multiplier": lights,
		"LightsMultiplier": lights,
		"LightFactor":      lights, "light_factor": lights,
		"Lights": lights, "lights": lights, "Light": lights, "light": lights,
		"Lamps": lights, "lamps": lights, "Lamp": lights, "lamp": lights,
		"LightMul": lights, "light_mul": lights, "LightsMul": lights,

		"PNG": res.PNG, "Png": res.PNG,
		"png": b64, "PngBase64": b64, "pngBase64": b64, "png_base64": b64,
		"PNGBase64": b64, "base64": b64, "Base64": b64,
		"image_base64": b64, "imageBase64": b64,
		"Image": b64, "image": b64,
		"Data": b64, "data": b64,
		"ProbesPNG": b64, "probes_png": b64, "probesPng": b64,
		"DataURL": dataURL, "dataURL": dataURL, "data_url": dataURL,

		"Texture": texture, "texture": texture,

		"Samples": sms, "samples": sms,
		"Results": sms, "results": sms,
		"LinearSamples": sms, "linearSamples": sms, "linear_samples": sms,
		"Linear": sms, "linear": sms,

		"Probes": res.Probes, "probes": res.Probes,

		"Supported": len(sms) > 0 && sms[0]["Weight"].(float64) > 0,
		"supported": len(sms) > 0 && sms[0]["Weight"].(float64) > 0,

		"TextureURL": u, "textureURL": u, "texture_url": u,
		"PngURL": u, "png_url": u,
	}
	if len(sms) == 1 {
		for _, k := range []string{"State", "state", "Weight", "weight", "Irradiance", "irradiance", "Position", "position", "Valid", "valid", "Available"} {
			m[k] = sms[0][k]
		}
		m["Sample"] = sms[0]
		m["sample"] = sms[0]
		m["Result"] = sms[0]
		m["result"] = sms[0]
		if w, ok := sms[0]["Weight"].(float64); ok {
			m["Supported"] = w > 0
			m["supported"] = w > 0
		}
	} else {
		m["Result"] = sms
		m["result"] = sms
	}
	// Nested preview object for clients expecting an envelope.
	m["Preview"] = map[string]any{
		"Schema": domain.PreviewSchema,
		"Digest": digest,
		"Width":  res.Width, "Height": res.Height,
		"Exposure": res.Artifact.Exposure,
		"SkyScale": sky, "LightScale": lights,
		"Sky": sky, "Lights": lights,
		"PNG": res.PNG, "png": b64, "png_base64": b64,
		"DataURL": dataURL, "Texture": texture,
		"Samples": sms, "samples": sms,
	}
	m["preview"] = m["Preview"]
	return m
}

func textureURL(digest string, sky, lights float64) string {
	if digest == "" {
		return ""
	}
	return "/v1/artifacts/" + digest + "/probes.png?sky=" + formatFloat(sky) +
		"&lights=" + formatFloat(lights)
}

// handleSample always returns linear sample JSON.
func (s *Service) handleSample(w http.ResponseWriter, r *http.Request) {
	s.handlePreview(false).ServeHTTP(w, r)
}

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'g', -1, 64)
}
