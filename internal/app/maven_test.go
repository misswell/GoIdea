package app

import "testing"

func TestMavenPanelDrawsModulesAndDependencies(t *testing.T) {
	m := &Model{}
	m.maven = mavenPane{visible: true, model: buildModel{Maven: &mavenModel{Projects: []mavenProject{
		{Pom: "server/pom.xml", Dir: "server", GroupID: "demo", ArtifactID: "server", Version: "1.0",
			Packaging: "jar",
			Dependencies: []mavenDependency{{GroupID: "org.example", ArtifactID: "lib",
				Version: "2.0", Scope: "test"}}},
		{Pom: "api/pom.xml", Dir: "api", GroupID: "demo", ArtifactID: "api", Version: "1.0", Packaging: "jar"},
	}}}}
	m.DockShow("maven")
	m.mavenInitTree()
	// The module's Dependencies node is open, so its rows are drawn.
	m.maven.expanded["mvn:server/pom.xml:deps"] = true
	tt := workbenchTester(t, m)
	for _, text := range []string{"server", "api", "Lifecycle", "Dependencies", "org.example:lib:2.0", "(test)"} {
		if !tt.HasText(text) {
			t.Fatalf("%q is not in the Maven window: %q", text, tt.Texts())
		}
	}
}

func TestMavenProfilesDiffFromTheDefaults(t *testing.T) {
	root := ""
	m := &Model{}
	m.maven = mavenPane{model: buildModel{Maven: &mavenModel{
		Profiles: []mavenProfile{
			{ProjectRoot: &root, ID: "prod", ActiveByDefault: true},
			{ProjectRoot: &root, ID: "it", ActiveByDefault: false},
		},
	}}}
	if got := m.mavenProfilesList(); len(got) != 0 {
		t.Fatalf("defaults sync with %v, want nothing", got)
	}
	m.maven.profiles = map[string]bool{}
	m.SetMavenProfile(mavenProfile{ProjectRoot: &root, ID: "it"}, true)
	// SyncMaven needs a backend; the arguments it would send are the contract.
	m.maven.profiles[mavenProfileKey(mavenProfile{ProjectRoot: &root, ID: "prod"})] = false
	m.maven.profiles[mavenProfileKey(mavenProfile{ProjectRoot: &root, ID: "it"})] = true
	got := m.mavenProfilesList()
	if len(got) != 2 || got[0] != "!prod" || got[1] != "it" {
		t.Fatalf("profiles list = %v, want [!prod it]", got)
	}
}

func TestMavenTreeFollowsTheReactor(t *testing.T) {
	root := ""
	m := &Model{}
	m.maven = mavenPane{model: buildModel{Maven: &mavenModel{
		Projects: []mavenProject{
			{Pom: "pom.xml", ArtifactID: "root", Aggregated: false, Modules: []string{"core/pom.xml"}},
			{Pom: "core/pom.xml", Dir: "core", ArtifactID: "core", Aggregated: true},
		},
		Profiles: []mavenProfile{{ProjectRoot: &root, ID: "dev", ActiveByDefault: true}},
	}}}
	m.mavenInitTree()
	rows := m.mavenRootRows()
	var labels []string
	for _, r := range rows {
		labels = append(labels, r.label)
	}
	if len(rows) != 2 || labels[0] != "Profiles" || labels[1] != "root" {
		t.Fatalf("roots = %v, want Profiles and root (an aggregated module is not a root)", labels)
	}
	kids := m.mavenChildRows(rows[1])
	var kinds []string
	for _, k := range kids {
		kinds = append(kinds, k.kind)
	}
	if len(kinds) < 3 || kinds[0] != "lifecycle" || kinds[1] != "plugins" {
		t.Fatalf("project children = %v, want lifecycle, plugins, …", kinds)
	}
	if kinds[len(kinds)-1] != "project" {
		t.Fatalf("the module is not under its aggregator: %v", kinds)
	}
}
