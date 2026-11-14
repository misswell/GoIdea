package app

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func solid(w, h int, c color.Color) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

func writePNG(t *testing.T, path string, img image.Image) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func TestGoldenIdenticalPasses(t *testing.T) {
	dir := t.TempDir()
	ref := filepath.Join(dir, "ref.png")
	img := solid(20, 20, color.RGBA{10, 20, 30, 255})
	writePNG(t, ref, img)
	res, err := CompareGolden(img, ref, "", GoldenSpec{})
	if err != nil || res.Missing {
		t.Fatalf("%v %+v", err, res)
	}
	if res.Diff != 0 || !res.Passed {
		t.Fatalf("identical frames differ: %+v", res)
	}
}

func TestGoldenMissingReferenceIsNotAPass(t *testing.T) {
	res, err := CompareGolden(solid(4, 4, color.Black), filepath.Join(t.TempDir(), "none.png"), "", GoldenSpec{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Missing || res.Passed {
		t.Fatalf("missing reference: %+v", res)
	}
}

func TestGoldenToleranceMasksAndDiffImage(t *testing.T) {
	dir := t.TempDir()
	refPath := filepath.Join(dir, "ref.png")
	writePNG(t, refPath, solid(40, 40, color.White))
	cur := solid(40, 40, color.White)
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			cur.Set(x, y, color.Black)
		}
	}
	// 16 of 1600 pixels differ: 1%, exactly at the default tolerance.
	res, err := CompareGolden(cur, refPath, filepath.Join(dir, "out", "diff.png"), GoldenSpec{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Diff != 16 || !res.Passed {
		t.Fatalf("diff = %+v", res)
	}
	if res.DiffPath == "" {
		t.Fatal("no difference image was written")
	}
	if _, err := os.Stat(res.DiffPath); err != nil {
		t.Fatalf("diff image: %v", err)
	}
	// A mask over the changed block makes the frames equal.
	res, err = CompareGolden(cur, refPath, "", GoldenSpec{Masks: []GoldenRect{{X: 0, Y: 0, W: 4, H: 4}}})
	if err != nil || res.Diff != 0 || !res.Passed {
		t.Fatalf("masked diff = %+v %v", res, err)
	}
	// A stricter tolerance fails the same frames.
	res, _ = CompareGolden(cur, refPath, "", GoldenSpec{MaxRatio: 0.001})
	if res.Passed {
		t.Fatalf("a strict tolerance passed: %+v", res)
	}
}

func TestGoldenSizeMismatchFails(t *testing.T) {
	ref := filepath.Join(t.TempDir(), "ref.png")
	writePNG(t, ref, solid(10, 10, color.White))
	res, err := CompareGolden(solid(12, 12, color.White), ref, "", GoldenSpec{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Passed || res.Ratio != 1 {
		t.Fatalf("a size change passed: %+v", res)
	}
}
