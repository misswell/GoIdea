package app

import "testing"

func TestScaffoldTemplateFilters(t *testing.T) {
	lookup := func(name string) string {
		if name == "name" {
			return "My Shop App"
		}
		return ""
	}
	cases := map[string]string{
		"{{name}}":           "My Shop App",
		"{{name|kebab}}":     "my-shop-app",
		"{{name|snake}}":     "my_shop_app",
		"{{name|pascal}}":    "MyShopApp",
		"{{name|lower}}":     "my shop app",
		"{{name|ident}}":     "myshopapp",
		"com.{{name|ident}}": "com.myshopapp",
	}
	for tmpl, want := range cases {
		if got := renderScaffoldTemplate(tmpl, lookup); got != want {
			t.Fatalf("%q = %q, want %q", tmpl, got, want)
		}
	}
}

func TestScaffoldFieldDefaultsChain(t *testing.T) {
	st := &newProjectState{
		name:   "My Shop",
		values: map[string]string{},
		fields: []scaffoldField{
			{ID: "group", Default: "com.example", Kind: scaffoldKind{Type: "text"}},
			{ID: "artifact", Default: "{{name|kebab}}", Kind: scaffoldKind{Type: "text"}},
			{ID: "package", Default: "{{group}}.{{artifact|ident}}", Kind: scaffoldKind{Type: "text"}},
			{ID: "release", Default: "21", Kind: scaffoldKind{Type: "choice"}},
		},
	}
	if got := st.scaffoldValue("artifact"); got != "my-shop" {
		t.Fatalf("artifact = %q", got)
	}
	if got := st.scaffoldValue("package"); got != "com.example.myshop" {
		t.Fatalf("package = %q", got)
	}
	pattern := `^[A-Za-z_][\w]*(\.[A-Za-z_][\w]*)*$`
	group := scaffoldField{ID: "group", Kind: scaffoldKind{Type: "text", Pattern: &pattern}}
	if errText := st.scaffoldFieldError(group); errText != "" {
		t.Fatalf("valid group rejected: %s", errText)
	}
	st.values["group"] = "1bad"
	if errText := st.scaffoldFieldError(group); errText == "" {
		t.Fatal("an invalid group passed")
	}
}

func TestScaffoldGeneratorSelection(t *testing.T) {
	m := &Model{}
	m.newProject = newProjectState{open: true, name: "demo", values: map[string]string{},
		catalog: []scaffoldGenerator{
			{ID: "empty", Name: "Empty Project", Git: true},
			{ID: "java", Name: "Java", Git: true, Fields: []scaffoldField{
				{ID: "build", Default: "maven", Kind: scaffoldKind{Type: "choice"}},
			}},
		}}
	m.SelectNewProjectGenerator("java")
	if m.newProject.generator != "java" || len(m.newProject.fields) != 1 {
		t.Fatalf("selection = %+v", m.newProject)
	}
	if !m.newProject.scaffoldReady() {
		t.Fatal("the generator path is not ready")
	}
	if got := m.newProject.scaffoldValue("build"); got != "maven" {
		t.Fatalf("build default = %q", got)
	}
}
