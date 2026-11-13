package app

import "testing"

func TestInvalidateParams(t *testing.T) {
	params := invalidateParams(nil)
	if opts, ok := params["options"]; !ok || opts != nil {
		t.Fatalf("restart params = %+v, want options nil", params)
	}
	params = invalidateParams(&InvalidateOptions{LocalHistory: true, BuildCaches: true})
	opts, ok := params["options"].(InvalidateOptions)
	if !ok {
		t.Fatalf("options = %T, want InvalidateOptions", params["options"])
	}
	if !opts.LocalHistory || opts.LanguageServers || !opts.BuildCaches {
		t.Fatalf("options = %+v, want Local History and build caches only", opts)
	}
}

func TestInvalidateDialogStagesAndCloses(t *testing.T) {
	m := &Model{}
	m.OpenInvalidateCaches()
	v := m.invalidateDlg
	if v == nil || !v.open {
		t.Fatalf("dialog = %+v, want open", v)
	}
	if v.localHistory || v.languageServers || v.buildCaches {
		t.Fatalf("defaults = %+v, want every optional cache off", v)
	}
	v.localHistory = true
	v.buildCaches = true
	if !v.open {
		t.Fatalf("staging closed the dialog")
	}
	m.closeInvalidateCaches()
	if m.invalidateDlg != nil {
		t.Fatalf("dialog stayed after Cancel: %+v", m.invalidateDlg)
	}
}
