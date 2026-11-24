package app

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/egoist/mygo"
)

const maxMarkdownImageBytes = 16 * 1024 * 1024

var (
	markdownImageExt    = regexp.MustCompile(`(?i)\.(png|jpe?g|gif|webp|avif|bmp|svg|ico|tiff?)$`)
	markdownInvalidName = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)
)

type markdownImageSource struct {
	name string
	data []byte
}

func isMarkdownPath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown", ".mdown", ".mkd", ".mkdn", ".mdwn", ".mdx", ".ronn", ".workbook":
		return true
	default:
		return false
	}
}

func isMarkdownImagePath(path string) bool {
	return markdownImageExt.MatchString(path)
}

// markdownImageName follows OpenIDEA's imageName: a dropped or copied file
// keeps its name, while a clipboard bitmap uses the configured timestamp
// pattern and a PNG extension.
func markdownImageName(source, pattern string, now time.Time) string {
	if source != "" {
		return source[strings.LastIndexAny(source, `/\`)+1:]
	}
	timestamp := now.Format("20060102-150405")
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		pattern = "image-{timestamp}"
	}
	pattern = strings.ReplaceAll(pattern, "{timestamp}", timestamp)
	pattern = markdownInvalidName.ReplaceAllString(pattern, "-")
	if pattern == "" {
		pattern = "image-" + timestamp
	}
	return pattern + ".png"
}

func markdownImageTarget(markdownPath, folder, name string) (string, error) {
	folder = strings.TrimSpace(folder)
	if folder == "" {
		folder = "assets"
	}
	if filepath.IsAbs(folder) || filepath.VolumeName(folder) != "" {
		return "", errors.New("image folder must be relative")
	}
	return filepath.Join(filepath.Dir(markdownPath), filepath.FromSlash(folder), name), nil
}

func markdownImageLink(markdownPath, target string) (string, error) {
	rel, err := filepath.Rel(filepath.Dir(markdownPath), target)
	if err != nil {
		return "", err
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return "![](" + strings.Join(parts, "/") + ")", nil
}

func writeMarkdownImage(markdownPath, folder, name string, data []byte) (string, error) {
	if len(data) == 0 {
		return "", errors.New("the image is empty")
	}
	if len(data) > maxMarkdownImageBytes {
		return "", errors.New("the image exceeds 16 MB")
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(filepath.Base(name), ext)
	if stem == "" {
		stem = "image"
	}
	for suffix := 0; ; suffix++ {
		candidateName := stem + ext
		if suffix > 0 {
			candidateName = fmt.Sprintf("%s-%d%s", stem, suffix, ext)
		}
		target, err := markdownImageTarget(markdownPath, folder, candidateName)
		if err != nil {
			return "", err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return "", err
		}
		f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		_, writeErr := f.Write(data)
		closeErr := f.Close()
		if writeErr != nil {
			_ = os.Remove(target)
			return "", writeErr
		}
		if closeErr != nil {
			_ = os.Remove(target)
			return "", closeErr
		}
		return markdownImageLink(markdownPath, target)
	}
}

// insertMarkdownImageSources saves native images beside the Markdown file and
// replaces the editor selection with the relative Markdown links. It returns
// false when the active tab is not an editable Markdown document or a source
// cannot be saved.
func (m *Model) insertMarkdownImageSources(t *Tab, sources []markdownImageSource) bool {
	if t == nil || t.state == nil || t.state.Buffer == nil || t.state.ReadOnly ||
		t.doc == nil || !isMarkdownPath(t.doc.Path()) || len(sources) == 0 {
		return false
	}
	prefs := m.prefs.General
	if m.settingsOpen {
		prefs = m.settingsDraft.generalPrefs()
	}
	links := make([]string, 0, len(sources))
	now := time.Now()
	for _, source := range sources {
		name := markdownImageName(source.name, prefs.markdownImageNamePattern(), now)
		link, err := writeMarkdownImage(t.doc.Path(), prefs.markdownImageFolder(), name, source.data)
		if err != nil {
			m.status = "Cannot insert image: " + err.Error()
			m.invalidate()
			return true
		}
		links = append(links, link)
	}
	t.state.Insert(strings.Join(links, "\n"))
	m.status = "Inserted image link"
	m.invalidate()
	return true
}

func (m *Model) pasteMarkdownImage(t *Tab) bool {
	if !m.activeEditorEditable() || t == nil || t.doc == nil || !isMarkdownPath(t.doc.Path()) {
		return false
	}
	data := mygo.Clipboard.ReadImage()
	if len(data) == 0 {
		return false
	}
	return m.insertMarkdownImageSources(t, []markdownImageSource{{data: data}})
}

func (m *Model) dropMarkdownImages(t *Tab, paths []string) bool {
	if len(paths) == 0 || t == nil || t.doc == nil || !isMarkdownPath(t.doc.Path()) {
		return false
	}
	sources := make([]markdownImageSource, 0, len(paths))
	for _, path := range paths {
		if !isMarkdownImagePath(path) {
			return false
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() > maxMarkdownImageBytes {
			return false
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return false
		}
		sources = append(sources, markdownImageSource{name: filepath.Base(path), data: data})
	}
	return m.insertMarkdownImageSources(t, sources)
}
