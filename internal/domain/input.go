package domain

type Request struct {
	Grid     Grid
	Lights   []Light
	Blocks   []Box
	Samples  []Vec
	Exposure float64
	// Sky supplies six fixed-axis linear sky colours. SkyOn records explicit
	// wire presence so an all-zero sky still enables the extended product.
	Sky   [6]Vec `json:"Sky"`
	SkyOn bool   `json:"-"`

	// Source-level aliases for the same six axis colours.
	SkyColors [6]Vec `json:"SkyColors"`
	SkyLight  [6]Vec `json:"SkyLight"`
	Ambient   [6]Vec `json:"Ambient"`

	// EmbedLinear forces the self-contained preview basis into the artifact
	// even when no sky is present (used by preview-aware clients).
	EmbedLinear bool `json:"-"`
}

// EffectiveSky returns the six axis colours from whichever alias was set.
func (r Request) EffectiveSky() SkyColors {
	if r.SkyOn || notZeroSky(r.Sky) {
		return SkyColors(r.Sky)
	}
	for _, candidate := range [][6]Vec{r.SkyColors, r.SkyLight, r.Ambient} {
		if notZeroSky(candidate) {
			return SkyColors(candidate)
		}
	}
	return SkyColors(r.Sky)
}

func notZeroSky(s [6]Vec) bool {
	for i := range s {
		for _, v := range s[i] {
			if v != 0 {
				return true
			}
		}
	}
	return false
}

// SkyEnabled reports whether axis sky lighting participates in the bake.
func (r Request) SkyEnabled() bool {
	if r.SkyOn {
		return true
	}
	return notZeroSky(r.Sky) || notZeroSky(r.SkyColors) || notZeroSky(r.SkyLight) || notZeroSky(r.Ambient)
}

type Grid struct {
	Origin Vec
	Step   float64
	Size   [3]int
}
type Light struct {
	Position, RGB Vec
	Radius, Power float64
}
type Box struct{ Min, Max Vec }
