package domain

import "encoding/json"

// PreviewSchema labels the self-contained relighting basis embedded in
// sky-enabled artifacts.
const PreviewSchema = "lantern-preview/v1"

// BasisProbe stores full-precision separated linear groups for one probe.
// Alias keys mirror the canonical group names so external preview clients
// written against nearby vocabularies can still locate the values.
type BasisProbe struct {
	Index      int   `json:"Index"`
	X          int   `json:"X"`
	Y          int   `json:"Y"`
	Z          int   `json:"Z"`
	Position   Vec   `json:"Position"`
	Valid      bool  `json:"Valid"`
	Light      Lobes `json:"Light"`
	Sky        Lobes `json:"Sky"`
	Irradiance Lobes `json:"Irradiance"`

	Lights Lobes `json:"Lights"`
	Lamp   Lobes `json:"Lamp"`
	Lamps  Lobes `json:"Lamps"`
	Direct Lobes `json:"Direct"`

	SkyLight Lobes `json:"SkyLight"`
	Ambient  Lobes `json:"Ambient"`
	Total    Lobes `json:"Total"`
	Original Lobes `json:"Original"`
}

// Basis is the artifact-only relighting base: every geometric fact and both
// original linear groups needed to re-render and re-sample without the
// original request.
type Basis struct {
	Schema     string       `json:"Schema"`
	Grid       Grid         `json:"Grid"`
	Axes       [6]string    `json:"Axes"`
	Exposure   float64      `json:"Exposure"`
	Width      int          `json:"TextureWidth"`
	Height     int          `json:"TextureHeight"`
	Blocks     []Box        `json:"Blocks"`
	Sky        SkyColors    `json:"Sky"`
	SkyEnabled bool         `json:"SkyEnabled"`
	ProbeCount int          `json:"ProbeCount"`
	Probes     []BasisProbe `json:"Probes"`
}

func buildBasis(r Request, probes []Probe) *Basis {
	g := r.Grid
	b := &Basis{
		Schema:     PreviewSchema,
		Grid:       g,
		Axes:       axisNames,
		Exposure:   r.Exposure,
		Width:      g.Size[0] * 6,
		Height:     g.Size[1] * g.Size[2],
		Blocks:     r.Blocks,
		Sky:        r.EffectiveSky(),
		SkyEnabled: r.SkyEnabled(),
		ProbeCount: len(probes),
		Probes:     make([]BasisProbe, 0, len(probes)),
	}
	for z := range g.Size[2] {
		for y := range g.Size[1] {
			for x := range g.Size[0] {
				p := probes[g.index(x, y, z)]
				rec := BasisProbe{
					Index: g.index(x, y, z), X: x, Y: y, Z: z,
					Position: p.Position, Valid: p.Valid,
					Light: p.Lamp, Sky: p.Sky, Irradiance: p.Irradiance,
					Lights: p.Lamp, Lamp: p.Lamp, Lamps: p.Lamp, Direct: p.Lamp,
					SkyLight: p.Sky, Ambient: p.Sky,
					Total: p.Irradiance, Original: p.Irradiance,
				}
				b.Probes = append(b.Probes, rec)
			}
		}
	}
	return b
}

// extendedMetadata keeps the original Metadata field order and appends the
// separated linear probe groups at top level plus a canonical Preview basis.
type extendedMetadata struct {
	Metadata
	Blocks      []Box        `json:"Blocks"`
	Sky         SkyColors    `json:"Sky"`
	SkyEnabled  bool         `json:"SkyEnabled"`
	EmbedLinear bool         `json:"EmbedLinear,omitempty"`
	Probes      []BasisProbe `json:"Probes"`
	Preview     *Basis       `json:"Preview,omitempty"`
}

func extendMetadata(r Request, m Metadata, probes []Probe) extendedMetadata {
	b := buildBasis(r, probes)
	return extendedMetadata{
		Metadata:    m,
		Blocks:      r.Blocks,
		Sky:         r.EffectiveSky(),
		SkyEnabled:  r.SkyEnabled(),
		EmbedLinear: true,
		Probes:      b.Probes,
		Preview:     b,
	}
}

// layoutEnvelope captures just enough of layout.json to locate a basis even
// when it was written by a slightly different vocabulary.
type layoutEnvelope struct {
	Grid     Grid    `json:"Grid"`
	Exposure float64 `json:"Exposure"`
	Blocks   []Box   `json:"Blocks"`

	Preview  json.RawMessage `json:"Preview"`
	Relight  json.RawMessage `json:"Relight"`
	Lighting json.RawMessage `json:"Lighting"`
	Basis    json.RawMessage `json:"Basis"`
	Rig      json.RawMessage `json:"Rig"`
	Base     json.RawMessage `json:"Base"`

	Probes json.RawMessage `json:"Probes"`
}
