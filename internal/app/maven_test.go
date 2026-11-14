package app

import "testing"

func TestMavenPanelDrawsModulesAndDependencies(t *testing.T) {
	m := &Model{}
	m.maven = mavenPane{visible: true, model: buildModel{Maven: &mavenModel{Projects: []mavenProject{
		{Dir: "server", GroupID: "demo", ArtifactID: "server", Version: "1.0", Packaging: "jar",
			Dependencies: []mavenDependency{{GroupID: "org.example", ArtifactID: "lib", Version: "2.0", Scope: "test"}}},
		{Dir: "api", GroupID: "demo", ArtifactID: "api", Version: "1.0", Packaging: "jar"},
	}}}}
	m.DockShow("maven")
	m.mavenToggle("server")
	tt := workbenchTester(t, m)
	for _, text := range []string{"Maven", "server", "api", "org.example:lib:2.0", "test"} {
		if !tt.HasText(text) {
			t.Fatalf("%q is not in the Maven window: %q", text, tt.Texts())
		}
	}
}
