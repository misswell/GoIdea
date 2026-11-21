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

func TestJavaServerStillReadyAfterInitialLoad(t *testing.T) {
	cases := []struct {
		name        string
		status      string
		serverRoot  string
		readyRoot   string
		projectRoot string
		want        bool
	}{
		{"ready", "ready", "/project", "", "/project", true},
		{"busy after ready", "busy", "/project", "/project", "/project", true},
		{"navigation busy after ready", "busy", "", "/project", "/project", true},
		{"busy without ready", "busy", "/project", "", "/project", false},
		{"busy from another project", "busy", "/other", "/project", "/other", false},
		{"restart", "starting", "/project", "/project", "/project", false},
	}
	for _, c := range cases {
		if got := javaServerStillReady(c.status, c.serverRoot, c.readyRoot, c.projectRoot); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}
