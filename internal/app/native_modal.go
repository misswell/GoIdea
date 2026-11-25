package app

// nativeModal is OpenIDEA's dialog layer. Its backdrop blocks the workbench
// without dimming it, its window uses the IDE surface colours, and the panel
// rises 4 DIPs over 140 ms. The helper follows ui.Modal's shape so every
// feature keeps its existing open/dismiss call sites.

import (
	"time"

	"github.com/egoist/mygo/ui"
)

const oiDialogMotion = 140 * time.Millisecond

// nativeModalDefault is set while a dialog's content is built. OpenIDEA
// routes Enter to a dialog's enabled default button wherever focus is.
var nativeModalDefault func()

func setNativeModalDefault(fn func()) { nativeModalDefault = fn }

// nativeModalElement is the panel nativeModal drew. Dismissed reports a click
// on the transparent backdrop or Escape, as ui.Modal does.
type nativeModalElement struct {
	ui.Element
	dismissed bool
}

func (e nativeModalElement) Dismissed() bool { return e.dismissed }

func nativeModal(c *ui.Context, open *bool, fn func()) nativeModalElement {
	if !*open {
		return nativeModalElement{}
	}
	oi := oiPaletteOf(c.Theme())
	width, height := c.Size()
	dismissed := false
	savedDefault := nativeModalDefault
	nativeModalDefault = nil
	var panel ui.Element
	ui.Overlay(c, func() {
		back := ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0).Center().Modal()
		// A transparent full-window backdrop blocks pointer events outside the
		// dialog and sends them to the same close path as OpenIDEA.
		if back.Clicked() || back.OverlayShortcut(0, ui.KeyEscape) {
			*open = false
			dismissed = true
		}
		back.Children(func() {
			panel = ui.Column(c).Role(ui.RoleDialog).MaxWidth(width-48).
				MaxHeight(height-48).Padding(20).Gap(12).Radius(6).
				Background(oi.elev).Border(1, oi.border).
				Shadow(0, 10, 30, 0, ui.RGBA(0, 0, 0, 0.30)).
				Transition(ui.ElementTransition{
					Duration: oiDialogMotion,
					Ease:     ui.EaseOut,
					Enter:    &ui.Motion{Y: 4, Scale: 0.995},
				})
			panel.Children(fn)
			if action := nativeModalDefault; action != nil &&
				panel.OverlayShortcut(0, ui.KeyEnter) {
				action()
			}
		})
	})
	nativeModalDefault = savedDefault
	return nativeModalElement{Element: panel, dismissed: dismissed}
}

// nativeSurfaceModal keeps the same blocking, transparent backdrop and rise
// motion for dialogs that draw their complete window surface themselves.
func nativeSurfaceModal(c *ui.Context, open *bool, fn func()) nativeModalElement {
	if !*open {
		return nativeModalElement{}
	}
	width, height := c.Size()
	dismissed := false
	savedDefault := nativeModalDefault
	nativeModalDefault = nil
	var panel ui.Element
	ui.Overlay(c, func() {
		back := ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0).Center().Modal()
		if back.Clicked() || back.OverlayShortcut(0, ui.KeyEscape) {
			*open = false
			dismissed = true
		}
		back.Children(func() {
			panel = ui.Column(c).Role(ui.RoleDialog).MaxWidth(width - 48).
				MaxHeight(height - 48).Transition(ui.ElementTransition{
				Duration: oiDialogMotion,
				Ease:     ui.EaseOut,
				Enter:    &ui.Motion{Y: 4, Scale: 0.995},
			})
			panel.Children(fn)
			if action := nativeModalDefault; action != nil &&
				panel.OverlayShortcut(0, ui.KeyEnter) {
				action()
			}
		})
	})
	nativeModalDefault = savedDefault
	return nativeModalElement{Element: panel, dismissed: dismissed}
}
