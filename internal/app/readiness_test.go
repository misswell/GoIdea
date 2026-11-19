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
