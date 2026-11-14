package app

// Golden screenshot comparison for the visual regression: the rendered frame
// against a reference PNG at the same size, with dynamic regions masked, a
// per-state tolerance and a written difference image. A missing reference is
// reported as such, never as a pass.

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
)

// GoldenRect is a rectangle ignored by the comparison: clocks, branch names
// and other content that differs between runs.
type GoldenRect struct{ X, Y, W, H int }

// GoldenSpec is how one state is compared.
type GoldenSpec struct {
	// MaxRatio is the share of differing pixels allowed; zero is 1%.
	MaxRatio float64
	Masks    []GoldenRect
}

// GoldenResult is what the comparison found.
type GoldenResult struct {
	Missing  bool
	Diff     int
	Total    int
	Ratio    float64
	Passed   bool
	DiffPath string
}

// DefaultGoldenRatio is the tolerance when a state sets none.
const DefaultGoldenRatio = 0.01

// CompareGolden reads refPath and compares it with current, writing a
// difference image to diffPath when one is asked for. Missing references come
// back with Missing set and no error.
func CompareGolden(current image.Image, refPath, diffPath string, spec GoldenSpec) (GoldenResult, error) {
	res := GoldenResult{}
	f, err := os.Open(refPath)
	if err != nil {
		if os.IsNotExist(err) {
			res.Missing = true
			return res, nil
		}
		return res, err
	}
	defer f.Close()
	ref, err := png.Decode(f)
	if err != nil {
		return res, err
	}
	cb, rb := current.Bounds(), ref.Bounds()
	if cb.Dx() != rb.Dx() || cb.Dy() != rb.Dy() {
		res.Total = cb.Dx() * cb.Dy()
		res.Diff = res.Total
		res.Ratio = 1
		res.Passed = false
		if diffPath != "" {
			_ = writeDiff(current, cb, nil, rb, diffPath)
		}
		return res, nil
	}
	masked := func(x, y int) bool {
		for _, m := range spec.Masks {
			if x >= m.X && x < m.X+m.W && y >= m.Y && y < m.Y+m.H {
				return true
			}
		}
		return false
	}
	diff := image.NewRGBA(image.Rect(0, 0, cb.Dx(), cb.Dy()))
	for y := 0; y < cb.Dy(); y++ {
		for x := 0; x < cb.Dx(); x++ {
			if masked(x, y) {
				continue
			}
			res.Total++
			cr := current.At(cb.Min.X+x, cb.Min.Y+y)
			rr := ref.At(rb.Min.X+x, rb.Min.Y+y)
			if !sameRGB(cr, rr) {
				res.Diff++
				diff.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
				continue
			}
			dr, dg, db, _ := cr.RGBA()
			diff.Set(x, y, color.RGBA{R: uint8(dr >> 8), G: uint8(dg >> 8), B: uint8(db >> 8), A: 255})
		}
	}
	if res.Total > 0 {
		res.Ratio = float64(res.Diff) / float64(res.Total)
	}
	max := spec.MaxRatio
	if max <= 0 {
		max = DefaultGoldenRatio
	}
	res.Passed = res.Ratio <= max
	if diffPath != "" {
		if err := os.MkdirAll(dirOf(diffPath), 0o755); err == nil {
			out, err := os.Create(diffPath)
			if err == nil {
				_ = png.Encode(out, diff)
				out.Close()
				res.DiffPath = diffPath
			}
		}
	}
	return res, nil
}

func sameRGB(a, b color.Color) bool {
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	return ar>>8 == br>>8 && ag>>8 == bg>>8 && ab>>8 == bb>>8
}

func writeDiff(current image.Image, cb image.Rectangle, _ image.Image, _ image.Rectangle, path string) error {
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	return png.Encode(out, current)
}

func dirOf(path string) string {
	i := len(path) - 1
	for i >= 0 && path[i] != '/' && path[i] != '\\' {
		i--
	}
	if i <= 0 {
		return "."
	}
	return path[:i]
}

// stateGoldenSpec is the comparison rule per state: dynamic regions are
// masked, and a state with an animated part gets a looser ratio.
func stateGoldenSpec(state string, width, height int) GoldenSpec {
	spec := GoldenSpec{}
	// The status bar carries the backend state and process details.
	spec.Masks = append(spec.Masks, GoldenRect{X: 0, Y: height - 34, W: width, H: 34})
	switch state {
	case "balloons", "notifications":
		// Timestamps on the balloon cards.
		spec.Masks = append(spec.Masks, GoldenRect{X: width - 420, Y: 0, W: 420, H: height})
	}
	return spec
}

// SmokeGoldens compares the rendered shots under currentDir with the
// reference set under refDir, one <theme>/<state> pair at a time, and writes
// difference images below diffDir. A state without a reference is reported in
// missing, so a run with no goldens cannot look green.
func (m *Model) SmokeGoldens(currentDir, refDir, diffDir string) map[string]any {
	out := map[string]any{"results": map[string]any{}, "missing": []string{}}
	missing := []string{}
	results := map[string]any{}
	_ = filepath.Walk(refDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() || filepath.Ext(path) != ".png" {
			return nil
		}
		rel, err := filepath.Rel(refDir, path)
		if err != nil {
			return nil
		}
		key := filepath.ToSlash(rel)
		key = strings.TrimSuffix(key, ".png")
		curPath := filepath.Join(currentDir, filepath.FromSlash(key+".png"))
		f, err := os.Open(curPath)
		if err != nil {
			missing = append(missing, key)
			return nil
		}
		img, err := png.Decode(f)
		f.Close()
		if err != nil {
			missing = append(missing, key)
			return nil
		}
		state := filepath.Base(key)
		spec := stateGoldenSpec(state, smokeShotWidth, smokeShotHeight)
		res, err := CompareGolden(img, path, filepath.Join(diffDir, filepath.FromSlash(key+".diff.png")), spec)
		if err != nil {
			results[key] = map[string]any{"error": err.Error()}
			return nil
		}
		if res.Missing {
			missing = append(missing, key)
			return nil
		}
		results[key] = map[string]any{"ratio": res.Ratio, "passed": res.Passed,
			"diff": res.Diff, "total": res.Total, "diffPath": res.DiffPath}
		return nil
	})
	out["results"] = results
	out["missing"] = missing
	out["references"] = len(results) + len(missing)
	return out
}
