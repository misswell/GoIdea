package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
	"mayoi/internal/vfs"
)

const pluginWorkspaceFileLimit int64 = 16 << 20

// pluginTextPosition is the LSP-style position plugin documents use.
type pluginTextPosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// pluginTextRange is a half-open range in a plugin document.
type pluginTextRange struct {
	Start pluginTextPosition `json:"start"`
	End   pluginTextPosition `json:"end"`
}

type pluginWorkspaceFileStat struct {
	IsDir bool  `json:"isDir"`
	Size  int64 `json:"size"`
	Mtime int64 `json:"mtime"`
}

func (h *goideaPluginHost) pluginOpenFile(value string, line, column int) {
	value = filepath.Clean(value)
	h.m.post(func() {
		if line <= 0 {
			h.m.openFile(value)
			return
		}
		h.m.openFileThen(value, func(tab *Tab) {
			if tab == nil || tab.state == nil {
				return
			}
			tab.state.GoTo(editor.Pos{
				Line: max(0, line-1),
				Col:  max(0, column-1),
			})
		})
	})
}

func (h *goideaPluginHost) pluginReadWorkspaceFile(value string) (string, error) {
	resolved, err := h.projectPath(value)
	if err != nil {
		return "", err
	}
	if doc := h.pluginOpenDocument(resolved); doc != nil {
		return doc.Text(), nil
	}
	if h.m.projectTreeUsesRemotePaths() {
		ctx, cancel := context.WithTimeout(h.m.remoteBaseContext(), 30*time.Second)
		defer cancel()
		request := h.m.nextRequest()
		raw, err := h.m.invoke(ctx, "plugin_workspace_read_text", map[string]any{
			"root": h.m.Root(), "path": resolved, "request": request,
		})
		if err != nil {
			if ctx.Err() != nil {
				h.m.cancelRequest(request)
			}
			return "", err
		}
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return "", err
		}
		return text, nil
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory", resolved)
	}
	if info.Size() > pluginWorkspaceFileLimit {
		return "", fmt.Errorf("%s is larger than %d MiB", resolved, pluginWorkspaceFileLimit>>20)
	}
	raw, err := os.ReadFile(resolved)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func (h *goideaPluginHost) pluginWriteWorkspaceFile(value, text string) error {
	if int64(len(text)) > pluginWorkspaceFileLimit {
		return fmt.Errorf("workspace.writeFile is limited to %d MiB", pluginWorkspaceFileLimit>>20)
	}
	resolved, err := h.projectPath(value)
	if err != nil {
		return err
	}
	doc := h.pluginOpenDocument(resolved)
	if doc != nil && doc.Dirty() {
		return fmt.Errorf("workspace.writeFile refused: %s has unsaved changes", resolved)
	}
	if h.m.projectTreeUsesRemotePaths() {
		ctx, cancel := context.WithTimeout(h.m.remoteBaseContext(), 30*time.Second)
		defer cancel()
		request := h.m.nextRequest()
		if _, err := h.m.invoke(ctx, "plugin_workspace_write_text", map[string]any{
			"root": h.m.Root(), "path": resolved, "text": text, "request": request,
		}); err != nil {
			if ctx.Err() != nil {
				h.m.cancelRequest(request)
			}
			return err
		}
		if doc != nil && doc.Dirty() {
			return fmt.Errorf("workspace.writeFile conflicted with a new edit in %s", resolved)
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return err
	}
	if _, err := vfs.WriteFile(resolved, []byte(text), 0o644); err != nil {
		return err
	}
	if doc != nil && doc.Dirty() {
		return fmt.Errorf("workspace.writeFile conflicted with a new edit in %s", resolved)
	}
	return nil
}

func (h *goideaPluginHost) pluginWorkspaceStat(value string) (any, error) {
	resolved, err := h.projectPath(value)
	if err != nil {
		return nil, err
	}
	if h.m.projectTreeUsesRemotePaths() {
		ctx, cancel := context.WithTimeout(h.m.remoteBaseContext(), 30*time.Second)
		defer cancel()
		request := h.m.nextRequest()
		raw, err := h.m.invoke(ctx, "plugin_workspace_stat", map[string]any{
			"root": h.m.Root(), "path": resolved, "request": request,
		})
		if err != nil {
			if ctx.Err() != nil {
				h.m.cancelRequest(request)
			}
			return nil, err
		}
		var stat *pluginWorkspaceFileStat
		if err := json.Unmarshal(raw, &stat); err != nil {
			return nil, err
		}
		if stat == nil {
			return nil, nil
		}
		return map[string]any{"isDir": stat.IsDir, "size": stat.Size, "mtime": stat.Mtime}, nil
	}
	info, err := os.Stat(resolved)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	size := int64(0)
	if info.Mode().IsRegular() {
		size = info.Size()
	}
	return map[string]any{
		"isDir": info.IsDir(),
		"size":  size,
		"mtime": info.ModTime().UnixMilli(),
	}, nil
}

func (h *goideaPluginHost) pluginFindWorkspaceFiles(glob string, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 1000
	}
	limit = max(1, min(limit, 4096))
	root := h.m.Root()
	if root == "" {
		return []string{}, nil
	}
	ctx, cancel := context.WithTimeout(h.m.remoteBaseContext(), 30*time.Second)
	defer cancel()
	request := h.m.nextRequest()
	raw, err := h.m.invoke(ctx, "plugin_workspace_find_files", map[string]any{
		"root": root, "glob": strings.TrimSpace(glob), "limit": limit, "request": request,
	})
	if err != nil {
		if ctx.Err() != nil {
			h.m.cancelRequest(request)
		}
		return nil, err
	}
	var paths []string
	if err := json.Unmarshal(raw, &paths); err != nil {
		return nil, err
	}
	if paths == nil {
		paths = []string{}
	}
	return paths, nil
}

func (h *goideaPluginHost) pluginDocuments() []map[string]any {
	var docs []*docbridge.Document
	h.m.post(func() {
		if h.m.reg != nil {
			docs = h.m.reg.Documents()
		}
	})
	out := make([]map[string]any, 0, len(docs))
	for _, doc := range docs {
		if doc == nil {
			continue
		}
		out = append(out, pluginDocumentData(doc))
	}
	return out
}

func pluginDocumentData(doc *docbridge.Document) map[string]any {
	snap := doc.Buffer().Snapshot()
	info := doc.Info()
	value := doc.Path()
	name := path.Base(strings.ReplaceAll(value, "\\", "/"))
	if name == "." || name == "/" {
		name = value
	}
	return map[string]any{
		"id":        doc.ID(),
		"path":      value,
		"name":      name,
		"language":  info.Language,
		"version":   snap.Version(),
		"dirty":     doc.Dirty(),
		"untitled":  value == "",
		"lineCount": snap.LineCount(),
	}
}

func (h *goideaPluginHost) pluginDocumentText(id uint32, span *pluginTextRange) (string, error) {
	doc := h.pluginDocumentByID(id)
	if doc == nil {
		return "", errors.New("the document is closed")
	}
	snap := doc.Buffer().Snapshot()
	start, end := 0, snap.Len()
	if span != nil {
		start = doc.ByteOffset(openideaPosition(span.Start))
		end = doc.ByteOffset(openideaPosition(span.End))
		if start > end {
			start, end = end, start
		}
	}
	start = max(0, min(start, snap.Len()))
	end = max(0, min(end, snap.Len()))
	return snap.Slice(start, end), nil
}

func (h *goideaPluginHost) pluginDocumentLine(id uint32, line int) (string, error) {
	doc := h.pluginDocumentByID(id)
	if doc == nil {
		return "", errors.New("the document is closed")
	}
	return doc.Buffer().Snapshot().Line(line), nil
}

func (h *goideaPluginHost) pluginSaveDocument(id uint32) error {
	doc := h.pluginDocumentByID(id)
	if doc == nil {
		return errors.New("the document is closed")
	}
	ctx, cancel := context.WithTimeout(h.m.remoteBaseContext(), 30*time.Second)
	defer cancel()
	return doc.Save(ctx)
}

func (h *goideaPluginHost) pluginOpenDocument(value string) *docbridge.Document {
	var doc *docbridge.Document
	h.m.post(func() {
		if h.m.reg != nil {
			doc = h.m.reg.Get(value)
			if doc == nil && !h.m.projectTreeUsesRemotePaths() {
				for _, candidate := range h.m.reg.Documents() {
					if samePluginLocalFile(candidate.Path(), value) {
						doc = candidate
						break
					}
				}
			}
		}
	})
	return doc
}

func (h *goideaPluginHost) pluginDocumentByID(id uint32) *docbridge.Document {
	var doc *docbridge.Document
	h.m.post(func() {
		if h.m.reg != nil {
			doc = h.m.reg.ByID(id)
		}
	})
	return doc
}

func openideaPosition(value pluginTextPosition) openidea.OutlinePos {
	return openidea.OutlinePos{
		Line: uint32(max(0, value.Line)),
		Col:  uint32(max(0, value.Character)),
	}
}

func samePluginLocalFile(left, right string) bool {
	leftReal, leftErr := filepath.EvalSymlinks(left)
	rightReal, rightErr := filepath.EvalSymlinks(right)
	if leftErr == nil && rightErr == nil && leftReal == rightReal {
		return true
	}
	leftInfo, leftErr := os.Stat(left)
	rightInfo, rightErr := os.Stat(right)
	return leftErr == nil && rightErr == nil && os.SameFile(leftInfo, rightInfo)
}
