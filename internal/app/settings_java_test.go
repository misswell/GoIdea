package app

import (
	"slices"
	"testing"

	"mayoi/internal/openidea"
)

func TestJavaSharedJDTLSIsDisabledOnlyForWorkspaceProjects(t *testing.T) {
	workspaceProject := &Model{
		workspace: &openidea.Workspace{},
		javaSettings: javaSettingsState{
			scope: "project",
		},
	}
	if !workspaceProject.javaSettingsSharedDisabled() {
		t.Fatal("workspace project allowed sharing JDT LS across builds")
	}
	workspaceProject.javaSettings.scope = "global"
	if workspaceProject.javaSettingsSharedDisabled() {
		t.Fatal("global Java settings disabled the shared JDT LS choice")
	}
	ordinaryProject := &Model{javaSettings: javaSettingsState{scope: "project"}}
	if ordinaryProject.javaSettingsSharedDisabled() {
		t.Fatal("ordinary project disabled the shared JDT LS choice")
	}
}

func TestJavaLanguageLevelsAndWritesMatchOpenIDEA(t *testing.T) {
	want := []string{
		"5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15",
		"16", "17", "18", "19", "20", "21", "22", "23", "24", "25",
	}
	if !slices.Equal(javaLanguageLevels, want) {
		t.Fatalf("Java levels = %v, want %v", javaLanguageLevels, want)
	}
	wantPHP := []string{
		"", "8.5", "8.4", "8.3", "8.2", "8.1", "8.0",
		"7.4", "7.3", "7.2", "7.1", "7.0", "5.6",
	}
	if !slices.Equal(phpLanguageLevels, wantPHP) {
		t.Fatalf("PHP levels = %v, want %v", phpLanguageLevels, wantPHP)
	}
	write := pendingJavaSettingWrites(JavaSettings{}, JavaSettings{LanguageLevel: "21"})
	if len(write) != 1 || write[0].key != "languages.java.languageLevel" ||
		write[0].value != `"21"` {
		t.Fatalf("Java level write = %+v", write)
	}
	write = pendingJavaSettingWrites(
		JavaSettings{LanguageLevel: "21"},
		JavaSettings{LanguageLevel: ""},
	)
	if len(write) != 1 || write[0].value != `""` {
		t.Fatalf("SDK-default write = %+v", write)
	}
}
