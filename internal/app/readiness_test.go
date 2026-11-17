package app

import "testing"

func TestReadinessReason(t *testing.T) {
	cases := []struct {
		name string
		r    Readiness
		want string
	}{
		{"backend", Readiness{}, "The backend is not connected"},
		{"project", Readiness{Backend: true}, "The project is not open yet"},
		{"importing", Readiness{Backend: true, Project: true, Importing: true},
			"The Java project is importing; wait for it to finish"},
		{"java failed", Readiness{Backend: true, Project: true, JavaDetail: "failed"},
			"Java language services are not ready (failed)"},
		{"build model", Readiness{Backend: true, Project: true, Java: true},
			"The build model is not loaded yet"},
		{"run", Readiness{Backend: true, Project: true, Java: true, BuildModel: true},
			"Run configurations are not loaded (project untrusted or still loading)"},
		{"ready", Readiness{Backend: true, Project: true, Java: true, BuildModel: true, Run: true}, ""},
	}
	for _, c := range cases {
		if got := c.r.readinessReason(); got != c.want {
			t.Errorf("%s: %q want %q", c.name, got, c.want)
		}
	}
}

func TestDebugReadinessWaitsForImport(t *testing.T) {
	// An import in progress is transient: Debug must wait, not refuse.
	m := &Model{}
	if gate, wait := m.debugReadiness(); gate != "" || wait {
		t.Fatalf("no snapshot: gate=%q wait=%v", gate, wait)
	}
	m.ready = &Readiness{Backend: true, Project: true, Importing: true}
	if gate, wait := m.debugReadiness(); gate != "" || !wait {
		t.Fatalf("importing: gate=%q wait=%v", gate, wait)
	}
	m.ready = &Readiness{Backend: true, Project: true, JavaDetail: "starting"}
	if gate, wait := m.debugReadiness(); gate != "" || !wait {
		t.Fatalf("starting: gate=%q wait=%v", gate, wait)
	}
	m.ready = &Readiness{Backend: true, Project: true, JavaDetail: "failed"}
	if gate, wait := m.debugReadiness(); gate == "" || wait {
		t.Fatalf("failed: gate=%q wait=%v", gate, wait)
	}
	// Debug does not depend on the build model or the run configurations.
	m.ready = &Readiness{Backend: true, Project: true, Java: true}
	if gate, wait := m.debugReadiness(); gate != "" || wait {
		t.Fatalf("java ready: gate=%q wait=%v", gate, wait)
	}
	m.ready = &Readiness{}
	if gate, wait := m.debugReadiness(); gate == "" || wait {
		t.Fatalf("backend down: gate=%q wait=%v", gate, wait)
	}
}
