package app

import (
	"time"

	"github.com/egoist/mygo/ui"
)

const doubleShiftStep = 300 * time.Millisecond

// doubleShiftState follows bare Shift press and release transitions. The
// window receives modifier changes by frame, which is enough to recognize two
// complete taps while rejecting holds, slow taps and combos with another
// modifier.
type doubleShiftState struct {
	initialized bool
	previous    ui.Modifiers
	pressedAt   time.Time
	releasedAt  time.Time
	taps        int
	blocked     bool
}

// observeDoubleShift reports a second Shift release made within 300 ms of
// each preceding step. It consumes that gesture, so the next call starts a
// fresh sequence.
func (s *doubleShiftState) observeDoubleShift(modifiers ui.Modifiers, now time.Time) bool {
	current := modifiers & (ui.Cmd | ui.Alt | ui.Ctrl | ui.Shift)
	if !s.initialized {
		s.initialized = true
		s.previous = current
		return false
	}
	previous := s.previous
	s.previous = current

	shift := current&ui.Shift != 0
	other := current&(ui.Cmd|ui.Alt|ui.Ctrl) != 0
	wasShift := previous&ui.Shift != 0

	if shift && other {
		s.pressedAt = time.Time{}
		s.releasedAt = time.Time{}
		s.taps = 0
		s.blocked = true
		return false
	}

	if shift {
		if !wasShift {
			if s.taps > 0 && (s.releasedAt.IsZero() || now.Sub(s.releasedAt) > doubleShiftStep) {
				s.taps = 0
			}
			s.pressedAt = now
		}
		return false
	}

	if !wasShift {
		s.blocked = false
		return false
	}

	fire := false
	if !s.blocked && !s.pressedAt.IsZero() && now.Sub(s.pressedAt) <= doubleShiftStep {
		if s.taps == 1 && !s.releasedAt.IsZero() && now.Sub(s.releasedAt) <= doubleShiftStep {
			fire = true
			s.taps = 0
		} else {
			s.taps = 1
		}
		s.releasedAt = now
	} else {
		s.taps = 0
		s.releasedAt = time.Time{}
	}
	s.pressedAt = time.Time{}
	s.blocked = false
	return fire
}
