package app

import (
	"encoding/json"
	"os"
	"path/filepath"
)

func (m *Model) templateRestorePath() string {
	if m == nil || m.cfg.SettingsPath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(m.cfg.SettingsPath), "templates.json")
}

func loadTemplatePrefs(path string) TemplatePrefs {
	defaults := defaultTemplatePrefs()
	if path == "" {
		return defaults
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return defaults
	}
	var prefs TemplatePrefs
	if json.Unmarshal(raw, &prefs) != nil {
		return defaults
	}
	return normalizeTemplatePrefs(prefs)
}

func (m *Model) templates() *TemplatePrefs {
	if m.templatePrefs == nil {
		prefs := loadTemplatePrefs(m.templateRestorePath())
		m.templatePrefs = &prefs
	}
	return m.templatePrefs
}

func (m *Model) saveTemplates(prefs TemplatePrefs) error {
	prefs = normalizeTemplatePrefs(prefs)
	raw, err := templateMarshal(prefs)
	if err != nil {
		return err
	}
	path := m.templateRestorePath()
	if path == "" {
		m.templatePrefs = &prefs
		return nil
	}
	if err := writeFileAtomic(path, append(raw, '\n')); err != nil {
		return err
	}
	m.templatePrefs = &prefs
	return nil
}

func (m *Model) effectiveFileTemplateScheme() FileTemplateScheme {
	root := ""
	var languages []openIDEALanguageInfo
	if m != nil {
		root = m.Root()
		languages = m.fileTypesSettings.languages
	}
	return m.templates().Files.languageTemplates(root, languages)
}

func (m *Model) projectFileTemplateScheme() FileTemplateScheme {
	return m.templates().Files.ensureProjectScheme(m.Root())
}
