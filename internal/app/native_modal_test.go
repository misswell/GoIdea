package app

import (
	"image/color"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestNativeModalUsesATransparentBackdropAndDefaultEnter(t *testing.T) {
	open := true
	entered := false
	background := ui.RGBA(42, 44, 49, 1)
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Fill().Background(background)
		nativeModal(c, &open, func() {
			setNativeModalDefault(func() { entered = true })
			ui.Text(c, "Dialog body")
		})
	}, 480, 320)
	tt.Frame()

	if !tt.HasText("Dialog body") {
		t.Fatalf("dialog body was not drawn: %v", tt.Texts())
	}
	got := tt.Image().RGBAAt(3, 3)
	want := color.RGBA{R: 42, G: 44, B: 49, A: 255}
	if got != want {
		t.Fatalf("backdrop = %#v, want undimmed #%#v", got, want)
	}
	tt.Key(ui.Modifiers(0), ui.KeyEnter)
	if !entered {
		t.Fatal("Enter did not reach the dialog's default action")
	}
	tt.Key(ui.Modifiers(0), ui.KeyEscape)
	if open {
		t.Fatal("Escape did not dismiss the dialog")
	}
}
