package domain

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

const CoarseSchema = "lantern-coarse/v1"
const CoarseEntry = "coarse.json"

// TierOptions controls optional coarse/fine delivery. ByteBudget < 0 means no
// byte cap. MaxLinearError is the permitted absolute pre-exposure channel error.
type TierOptions struct {
	Requested      bool
	ByteBudget     int
	MaxLinearError float64
	ErrorSet       bool
}

// UnlimitedBudget disables the coarse lighting byte cap.
const UnlimitedBudget = -1

// Accounting sizes the unpacked data actually used to reconstruct lighting.
// Grid specification and solid boxes are deliberately not included.
type Accounting struct {
	Colors             int `json:"Colors"`
	Coefficients       int `json:"Coefficients"`
	Lookup             int `json:"Lookup"`
	Total              int `json:"Total"`
	Budget             int `json:"Budget,omitempty"`
	ColorsBytes        int `json:"ColorsBytes"`
	CoefficientsBytes  int `json:"CoefficientsBytes"`
	LookupBytes        int `json:"LookupBytes"`
	TotalBytes         int `json:"TotalBytes"`
	LightingTotalBytes int `json:"LightingTotalBytes"`
	BudgetBytes        int `json:"BudgetBytes,omitempty"`
}

func makeAccounting(colors, coefficients, lookup, budget int) Accounting {
	total := colors + coefficients + lookup
	return Accounting{
		Colors: colors, Coefficients: coefficients, Lookup: lookup, Total: total, Budget: budget,
		ColorsBytes: colors, CoefficientsBytes: coefficients, LookupBytes: lookup,
		TotalBytes: total, LightingTotalBytes: total, BudgetBytes: budget,
	}
}

// CoarseEncoding is the compact, directly decodable lighting payload.
type CoarseEncoding struct {
	Mode         string    `json:"Mode"`
	Packing      string    `json:"Packing"`
	Scale        []float64 `json:"Scale,omitempty"`
	CodeWidths   []int     `json:"CodeWidths,omitempty"`
	IndexWidth   int       `json:"IndexWidth,omitempty"`
	ColorsBase64 string    `json:"ColorsBase64"`
	LookupBase64 string    `json:"LookupBase64,omitempty"`
}

// CoarseDocument is the self-contained coarse runtime resource. The grid and
// boxes let the engine reproduce state and support offline; only the encoding
// arrays enter ByteAccounting.
type CoarseDocument struct {
	Schema         string         `json:"Schema"`
	Format         string         `json:"Format"`
	Grid           Grid           `json:"Grid"`
	Blocks         []Box          `json:"Blocks"`
	ValidProbes    int            `json:"ValidProbes"`
	FaceCount      int            `json:"FaceCount"`
	Tolerance      float64        `json:"MaxLinearErrorAllowed"`
	ObservedError  float64        `json:"MaxLinearErrorObserved"`
	ZeroPreserving bool           `json:"ReferenceZeroChannelsRemainZero"`
	Encoding       CoarseEncoding `json:"Encoding"`
	ByteAccounting Accounting     `json:"ByteAccounting"`
}

// CoarseBudgetError reports that no representation meeting the error tolerance
// fits the requested unpacked lighting byte cap.
type CoarseBudgetError struct {
	Budget      int
	Required    int
	Tolerance   float64
	BestError   float64
	ValidProbes int
}

func (e *CoarseBudgetError) Error() string {
	return fmt.Sprintf("coarse tier generation failed: budget infeasible: cannot guarantee linear error %g within byte budget %d (unpacked lighting data); minimum required %d bytes (best observed error %.3g over %d valid probes)",
		e.Tolerance, e.Budget, e.Required, e.BestError, e.ValidProbes)
}

// AsCoarseBudgetError extracts the structured feasibility error.
func AsCoarseBudgetError(err error) (*CoarseBudgetError, bool) {
	var target *CoarseBudgetError
	if errors.As(err, &target) {
		return target, true
	}
	return nil, false
}

type packedScheme struct {
	mode       string
	scale      [3]float64
	width      [3]int
	colors     []byte
	lookup     []byte
	indexWidth int
	colorsN    int // number of indexed color entries (F direct or K palette)
	coeffBytes int
	obsError   float64
}

func widthFor(maxCode int64) int {
	switch {
	case maxCode <= 0xff:
		return 1
	case maxCode <= 0xffff:
		return 2
	case maxCode <= 0xffffffff:
		return 4
	default:
		return 8
	}
}

func putUint(buf []byte, offset int, v uint64, width int) {
	var tmp [8]byte
	binary.LittleEndian.PutUint64(tmp[:], v)
	copy(buf[offset:offset+width], tmp[:width])
}

func getUint(buf []byte, offset, width int) uint64 {
	var tmp [8]byte
	copy(tmp[:width], buf[offset:offset+width])
	return binary.LittleEndian.Uint64(tmp[:])
}

// collectFaces returns valid probe indices in canonical order and their face
// colors (six per valid probe).
func collectFaces(probes []Probe) ([]int, []Vec) {
	indices := make([]int, 0, len(probes))
	faces := make([]Vec, 0, 6*len(probes))
	for i := range probes {
		if !probes[i].Valid {
			continue
		}
		indices = append(indices, i)
		for _, c := range probes[i].Irradiance {
			faces = append(faces, c)
		}
	}
	return indices, faces
}

// encodeCoarse builds the document and verifies the error and byte guarantees.
func encodeCoarse(r Request, probes []Probe, opts TierOptions) (*CoarseDocument, error) {
	validIndices, faces := collectFaces(probes)
	F := len(faces)
	V := len(validIndices)
	budget := opts.ByteBudget
	eps := opts.MaxLinearError

	doc := &CoarseDocument{
		Schema:         CoarseSchema,
		Format:         CoarseSchema,
		Grid:           r.Grid,
		Blocks:         r.Blocks,
		ValidProbes:    V,
		FaceCount:      F,
		Tolerance:      eps,
		ZeroPreserving: true,
	}

	// Empty volume: no lighting bytes are required.
	if F == 0 {
		doc.Encoding = CoarseEncoding{Mode: "empty-v1", Packing: "no valid probes"}
		doc.ByteAccounting = makeAccounting(0, 0, 0, budget)
		if budget != UnlimitedBudget && 0 > budget {
			return nil, &CoarseBudgetError{Budget: budget, Required: 0, Tolerance: eps, ValidProbes: V}
		}
		return doc, nil
	}

	var best *packedScheme
	if eps <= 0 || eps < 1e-9 {
		best = buildExactScheme(faces)
	} else {
		best = buildQuantizedScheme(faces, eps)
		if best == nil {
			best = buildExactScheme(faces)
		}
	}

	colorsBytes := len(best.colors)
	lookupBytes := len(best.lookup)
	coeffBytes := best.coeffBytes
	total := colorsBytes + coeffBytes + lookupBytes
	doc.ObservedError = best.obsError
	doc.Encoding = CoarseEncoding{
		Mode:         best.mode,
		Packing:      "RGB planes in channel order; little-endian unsigned codes; " + best.packingHint(),
		Scale:        []float64{best.scale[0], best.scale[1], best.scale[2]},
		CodeWidths:   []int{best.width[0], best.width[1], best.width[2]},
		IndexWidth:   best.indexWidth,
		ColorsBase64: base64.StdEncoding.EncodeToString(best.colors),
		LookupBase64: base64.StdEncoding.EncodeToString(best.lookup),
	}
	doc.ByteAccounting = makeAccounting(colorsBytes, coeffBytes, lookupBytes, budget)

	if budget != UnlimitedBudget && total > budget {
		return nil, &CoarseBudgetError{
			Budget: budget, Required: total, Tolerance: eps,
			BestError: best.obsError, ValidProbes: V,
		}
	}
	return doc, nil
}

func (s *packedScheme) packingHint() string {
	switch s.mode {
	case "direct-codes-v1":
		return "ColorsBase64 is plane[channel][face]."
	case "palette-codes-v1":
		return "ColorsBase64 is plane[channel][palette entry]; LookupBase64 is per-face palette indices."
	default:
		return "ColorsBase64 is plane[channel][face] of float64 bits."
	}
}

func buildExactScheme(faces []Vec) *packedScheme {
	F := len(faces)
	buf := make([]byte, 24*F) // 3 channels * F * 8
	off := 0
	var obs float64
	for ch := 0; ch < 3; ch++ {
		for f := 0; f < F; f++ {
			binary.LittleEndian.PutUint64(buf[off:off+8], math.Float64bits(faces[f][ch]))
			off += 8
		}
	}
	return &packedScheme{
		mode: "exact-float64-v1", colors: buf, coeffBytes: 24,
		scale: [3]float64{1, 1, 1},
		width: [3]int{8, 8, 8}, obsError: obs,
	}
}

// buildQuantizedScheme searches near-maximal safe scales and returns the
// smallest unpacked representation whose verified channel error is within eps.
func buildQuantizedScheme(faces []Vec, eps float64) *packedScheme {
	F := len(faces)
	// Descending scale multipliers (scale = m*eps). Rounding error <= scale/2,
	// so m must stay below 2. Empirical verification closes the float gap.
	multipliers := []float64{1.999, 1.995, 1.99, 1.98, 1.95, 1.9, 1.8, 1.6, 1.3, 1.0, 0.75, 0.5}
	// Keep a safety cushion under the requested epsilon so that query-time
	// convex recombination and float rounding cannot cross the boundary.
	guard := eps*1e-6 + 1e-12
	limit := eps - guard
	if limit < 0 {
		limit = eps
	}
	var best *packedScheme
	bestTotal := math.MaxInt
	for _, m := range multipliers {
		s := m * eps
		codes := [3][]int64{make([]int64, F), make([]int64, F), make([]int64, F)}
		var mx [3]int64
		var obs float64
		for ch := 0; ch < 3; ch++ {
			for f := 0; f < F; f++ {
				v := faces[f][ch]
				q := int64(math.Round(v / s))
				if q < 0 {
					q = 0
				}
				codes[ch][f] = q
				if q > mx[ch] {
					mx[ch] = q
				}
				recon := float64(q) * s
				if e := math.Abs(recon - v); e > obs {
					obs = e
				}
			}
		}
		if obs > limit {
			continue
		}
		width := [3]int{widthFor(mx[0]), widthFor(mx[1]), widthFor(mx[2])}
		direct := packDirect(codes, width, F)
		directTotal := len(direct) + 24 // three float64 scale coefficients

		candidate := &packedScheme{
			mode: "direct-codes-v1", scale: [3]float64{s, s, s}, width: width,
			colors: direct, coeffBytes: 24, obsError: obs,
		}
		total := directTotal

		if pal := tryPalette(codes, width, F, s, obs); pal != nil {
			palTotal := len(pal.colors) + len(pal.lookup) + 24
			if palTotal < total {
				candidate = pal
				total = palTotal
			}
		}
		if total < bestTotal {
			bestTotal = total
			best = candidate
		}
	}
	return best
}

func packDirect(codes [3][]int64, width [3]int, F int) []byte {
	total := F * (width[0] + width[1] + width[2])
	buf := make([]byte, total)
	off := 0
	for ch := 0; ch < 3; ch++ {
		for f := 0; f < F; f++ {
			putUint(buf, off, uint64(codes[ch][f]), width[ch])
			off += width[ch]
		}
	}
	return buf
}

// tryPalette deduplicates quantized RGB triples.
func tryPalette(codes [3][]int64, width [3]int, F int, scale, obs float64) *packedScheme {
	type key [3]int64
	keys := make([]key, 0, F)
	indexByKey := make(map[key]int, F)
	indices := make([]int, F)
	for f := 0; f < F; f++ {
		k := key{codes[0][f], codes[1][f], codes[2][f]}
		if idx, ok := indexByKey[k]; ok {
			indices[f] = idx
		} else {
			idx := len(keys)
			indexByKey[k] = idx
			keys = append(keys, k)
			indices[f] = idx
		}
	}
	K := len(keys)
	if K >= 1<<32 {
		return nil
	}
	indexWidth := widthFor(int64(K - 1))
	if K == 1 {
		indexWidth = 1
	}
	colorBytes := K * (width[0] + width[1] + width[2])
	lookupBytes := F * indexWidth
	colors := make([]byte, colorBytes)
	off := 0
	for ch := 0; ch < 3; ch++ {
		for k := 0; k < K; k++ {
			putUint(colors, off, uint64(keys[k][ch]), width[ch])
			off += width[ch]
		}
	}
	lookup := make([]byte, lookupBytes)
	off = 0
	for f := 0; f < F; f++ {
		putUint(lookup, off, uint64(indices[f]), indexWidth)
		off += indexWidth
	}
	return &packedScheme{
		mode: "palette-codes-v1", scale: [3]float64{scale, scale, scale}, width: width,
		colors: colors, lookup: lookup, indexWidth: indexWidth,
		colorsN: K, coeffBytes: 24, obsError: obs,
	}
}
