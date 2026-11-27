package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"example.local/lantern-lattice/internal/domain"
	"io"
	"mime"
	"net/http"
)

// wireRequest is the accepted build document. Standard fields keep their
// strict typed decoding; sky configuration accepts a broad set of keys and
// shapes via raw messages. Every field here is "known" to
// DisallowUnknownFields, while genuinely unknown keys still fail with 400.
type wireRequest struct {
	Grid     domain.Grid    `json:"Grid"`
	Lights   []domain.Light `json:"Lights"`
	Blocks   []domain.Box   `json:"Blocks"`
	Samples  []domain.Vec   `json:"Samples"`
	Exposure float64        `json:"Exposure"`

	Sky            json.RawMessage `json:"Sky"`
	SkyColors      json.RawMessage `json:"SkyColors"`
	SkyColor       json.RawMessage `json:"SkyColor"`
	SkyLights      json.RawMessage `json:"SkyLights"`
	SkyLight       json.RawMessage `json:"SkyLight"`
	SkyLightColors json.RawMessage `json:"SkyLightColors"`
	SkyIrradiance  json.RawMessage `json:"SkyIrradiance"`
	SkyTint        json.RawMessage `json:"SkyTint"`
	AxisSky        json.RawMessage `json:"AxisSky"`
	AxisColors     json.RawMessage `json:"AxisColors"`
	Ambient        json.RawMessage `json:"Ambient"`
	AmbientLight   json.RawMessage `json:"AmbientLight"`
	Hemisphere     json.RawMessage `json:"Hemisphere"`
	Daylight       json.RawMessage `json:"Daylight"`

	EmbedLinear   json.RawMessage `json:"EmbedLinear"`
	Linear        json.RawMessage `json:"Linear"`
	SelfContained json.RawMessage `json:"SelfContained"`
	Preview       json.RawMessage `json:"Preview"`

	// Tolerated wrapper objects that may carry the sky block.
	Lighting    json.RawMessage `json:"Lighting"`
	Rig         json.RawMessage `json:"Rig"`
	Options     json.RawMessage `json:"Options"`
	Settings    json.RawMessage `json:"Settings"`
	Config      json.RawMessage `json:"Config"`
	Scene       json.RawMessage `json:"Scene"`
	Environment json.RawMessage `json:"Environment"`
	Env         json.RawMessage `json:"Env"`
	World       json.RawMessage `json:"World"`
}

func decode(w http.ResponseWriter, r *http.Request) (domain.Request, int, error) {
	var input domain.Request
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return input, 415, errors.New("expected application/json")
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 3<<20))
	d.DisallowUnknownFields()
	var wire wireRequest
	if err := d.Decode(&wire); err != nil {
		return input, decodeStatus(err), errors.New("invalid request document")
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return input, decodeStatus(err), errors.New("expected exactly one JSON object")
	}

	input = domain.Request{
		Grid:     wire.Grid,
		Lights:   wire.Lights,
		Blocks:   wire.Blocks,
		Samples:  wire.Samples,
		Exposure: wire.Exposure,
	}

	skyRaw := firstRaw(wire.Sky, wire.SkyColors, wire.SkyColor, wire.SkyLights,
		wire.SkyLight, wire.SkyLightColors, wire.SkyIrradiance, wire.SkyTint,
		wire.AxisSky, wire.AxisColors, wire.Ambient, wire.AmbientLight,
		wire.Hemisphere, wire.Daylight)
	if skyRaw == nil {
		skyRaw = skyFromWrappers(firstRaw(wire.Lighting, wire.Rig, wire.Options,
			wire.Settings, wire.Config, wire.Scene, wire.Environment, wire.Env, wire.World))
	}
	if skyRaw != nil {
		colors, set, perr := domain.ParseSky(skyRaw)
		if perr != nil {
			var se *domain.SkyError
			status := 422
			if errors.As(perr, &se) && se.BadRequest {
				status = 400
			}
			return input, status, perr
		}
		if set {
			input.Sky = colors
			input.SkyOn = true
		}
	}

	input.EmbedLinear = rawTruthy(firstRaw(wire.EmbedLinear, wire.Linear, wire.SelfContained)) ||
		rawTruthy(wire.Preview)

	return input, 0, nil
}

func firstRaw(msgs ...json.RawMessage) json.RawMessage {
	for _, m := range msgs {
		if len(bytes.TrimSpace(m)) > 0 && string(bytes.TrimSpace(m)) != "null" {
			return m
		}
	}
	return nil
}

// skyFromWrappers finds a sky block nested one level inside a lighting/options
// wrapper document.
func skyFromWrappers(wrapper json.RawMessage) json.RawMessage {
	if wrapper == nil {
		return nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(wrapper, &m); err != nil {
		return nil
	}
	for k, v := range m {
		nk := normKey(k)
		if nk == "sky" || nk == "skycolors" || nk == "skylight" || nk == "skylights" ||
			nk == "ambient" || nk == "ambientlight" || nk == "hemisphere" || nk == "daylight" {
			return v
		}
	}
	// One more level for e.g. {"Lighting":{"Rig":{"Sky":...}}}.
	for _, v := range m {
		if nested := skyFromWrappers(v); nested != nil {
			return nested
		}
	}
	return nil
}

// rawTruthy reports whether an option field asks for the embedded basis.
// Boolean true, a non-empty object or array count as enabled.
func rawTruthy(m json.RawMessage) bool {
	m = bytes.TrimSpace(m)
	if len(m) == 0 || string(m) == "null" || string(m) == "false" || string(m) == "0" {
		return false
	}
	if string(m) == "true" {
		return true
	}
	var v any
	if err := json.Unmarshal(m, &v); err != nil {
		return false
	}
	switch t := v.(type) {
	case bool:
		return t
	case map[string]any:
		if e, ok := t["EmbedLinear"].(bool); ok {
			return e
		}
		if e, ok := t["Linear"].(bool); ok {
			return e
		}
		if e, ok := t["Enabled"].(bool); ok {
			return e
		}
		return len(t) > 0
	default:
		return true
	}
}

func decodeStatus(err error) int {
	var size *http.MaxBytesError
	if errors.As(err, &size) {
		return 413
	}
	return 400
}
