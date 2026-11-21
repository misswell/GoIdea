package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestDoubleShiftDetection(t *testing.T) {
	var state doubleShiftState
	start := time.Unix(1, 0)
	step := func(at time.Duration, modifiers ui.Modifiers) bool {
		return state.observeDoubleShift(modifiers, start.Add(at))
	}
	if step(0, 0) || step(10*time.Millisecond, ui.Shift) ||
		step(50*time.Millisecond, 0) ||
		step(200*time.Millisecond, ui.Shift) {
		t.Fatal("double Shift fired before its second release")
	}
	if !step(250*time.Millisecond, 0) {
		t.Fatal("two Shift taps within 300 ms did not fire")
	}

	state = doubleShiftState{}
	state.observeDoubleShift(0, start)
	state.observeDoubleShift(ui.Shift, start.Add(10*time.Millisecond))
	state.observeDoubleShift(0, start.Add(50*time.Millisecond))
	if state.observeDoubleShift(ui.Shift, start.Add(400*time.Millisecond)) ||
		state.observeDoubleShift(0, start.Add(440*time.Millisecond)) {
		t.Fatal("a slow second Shift tap fired")
	}

	state = doubleShiftState{}
	state.observeDoubleShift(0, start)
	state.observeDoubleShift(ui.Shift, start.Add(10*time.Millisecond))
	state.observeDoubleShift(ui.Shift|ui.Cmd, start.Add(20*time.Millisecond))
	state.observeDoubleShift(ui.Cmd, start.Add(30*time.Millisecond))
	state.observeDoubleShift(0, start.Add(40*time.Millisecond))
	state.observeDoubleShift(ui.Shift, start.Add(50*time.Millisecond))
	if state.observeDoubleShift(0, start.Add(60*time.Millisecond)) {
		t.Fatal("Shift pressed with Cmd counted as one of two taps")
	}
}

func TestDoubleShiftOpensSearchEverywhereOffscreen(t *testing.T) {
	m := &Model{}
	tt := ui.NewTester(m.View, 900, 700)
	tt.Frame()
	for range 2 {
		tt.HoldModifiers(ui.Shift)
		tt.HoldModifiers(0)
	}
	if m.se == nil || m.se.tab != seTabAll {
		t.Fatalf("double Shift did not open Search Everywhere: %+v", m.se)
	}
}
