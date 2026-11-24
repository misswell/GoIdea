package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

const remoteImportChunkSize = 256 << 10

// importLocalTreeFiles copies client files into the connected remote project.
// Files stream through bounded OpenIDEA RPC chunks under private temporary
// names. A final exclusive rename publishes each file, and folders are created
// exclusively at every level so concurrent writers cannot merge an import.
func (m *Model) importLocalTreeFiles(paths []string, targetDir string) {
	paths = cleanLocalImportPaths(paths)
	if len(paths) == 0 || targetDir == "" {
		m.notifyInfo("project.paste", "No files on the clipboard", "")
		return
	}
	if !m.projectTreeIsDir(targetDir) {
		m.notifyError("project.paste", "Cannot import here", "Choose a remote folder.")
		return
	}
	state := &m.remote
	if state.importBusy {
		m.notifyInfo("project.paste", "A remote import is already running", "Cancel it before starting another import.")
		return
	}

	root := m.Root()
	request := m.nextRequest()
	base, cancelOperation := context.WithCancel(m.remoteBaseContext())
	ctx, cancelTimeout := context.WithTimeout(base, 30*time.Minute)
	state.importBusy = true
	state.importLabel = fmt.Sprintf("Importing %d local item(s)…", len(paths))
	state.importRequest = request
	state.importCancel = cancelOperation
	done := make(chan struct{})
	state.importDone = done
	m.status = state.importLabel
	m.DockShow(remoteDevelopmentWindow)
	m.invalidate()

	go func() {
		defer close(done)
		defer cancelTimeout()
		defer cancelOperation()
		var imported []string
		var skipped []string
		for _, source := range paths {
			name := filepath.Base(source)
			if name == "." || name == string(filepath.Separator) || name == "" {
				skipped = append(skipped, source+": invalid file name")
				continue
			}
			destination := path.Join(targetDir, filepath.ToSlash(name))
			created, err := m.importLocalTreeEntry(ctx, source, destination, request, &skipped)
			if err != nil {
				if !created && remoteImportConflict(err) {
					skipped = append(skipped, source+": "+err.Error())
					continue
				}
				if created {
					m.cleanupRemoteImport(destination)
				}
				cancelled := remoteImportCancelled(ctx, err)
				m.post(func() {
					if m.Root() != root {
						return
					}
					state.importBusy = false
					state.importLabel = ""
					state.importRequest = 0
					state.importCancel = nil
					m.refreshTree(targetDir)
					if cancelled {
						m.status = "Remote import cancelled"
						m.notifyInfo("project.paste", "Remote import cancelled",
							"Files already committed were kept and the current item was removed.")
					} else {
						m.status = ""
						m.notifyError("project.paste", "Could not import "+name, err.Error())
					}
					m.invalidate()
				})
				return
			}
			if created {
				imported = append(imported, destination)
			}
		}
		m.post(func() {
			if m.Root() != root {
				return
			}
			state.importBusy = false
			state.importLabel = ""
			state.importRequest = 0
			state.importCancel = nil
			m.refreshTree(targetDir)
			if len(imported) > 0 && m.tree != nil {
				m.tree.Select(imported[0])
				m.notifyVCSFileOperation("created", imported)
			}
			switch len(imported) {
			case 0:
				m.status = ""
			case 1:
				m.status = "Imported " + m.projectTreePathBase(imported[0])
			default:
				m.status = fmt.Sprintf("Imported %d items", len(imported))
			}
			if len(skipped) > 0 {
				m.notifyWarning(
					"project.paste",
					"Some items were not imported",
					strings.Join(skipped, "\n"),
				)
			}
			m.invalidate()
		})
	}()
}

// CancelRemoteImport stops the current streaming import when one is active.
func (m *Model) CancelRemoteImport() {
	m.cancelRemoteImport()
}

// cancelRemoteImport starts cancellation and returns the channel that closes
// only after the worker has removed its temporary entry. A project transition
// must wait for that cleanup before it changes the Manager's transport.
func (m *Model) cancelRemoteImport() <-chan struct{} {
	state := &m.remote
	if !state.importBusy {
		return nil
	}
	request := state.importRequest
	done := state.importDone
	state.importLabel = "Cancelling remote import…"
	m.invalidate()
	if state.importCancel != nil {
		state.importCancel()
	}
	if request == 0 {
		return done
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_, _ = m.invoke(ctx, "cancel_request", map[string]any{"id": request})
	}()
	return done
}

func waitRemoteImportDone(ctx context.Context, done <-chan struct{}) error {
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("remote import cleanup did not finish: %w", ctx.Err())
	}
}

func remoteImportCancelled(ctx context.Context, err error) bool {
	return errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(ctx.Err(), context.Canceled) ||
		errors.Is(ctx.Err(), context.DeadlineExceeded)
}

func remoteImportConflict(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "already exists") || strings.Contains(message, "file exists")
}

func cleanLocalImportPaths(paths []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(paths))
	for _, value := range paths {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		absolute, err := filepath.Abs(value)
		if err != nil {
			continue
		}
		absolute = filepath.Clean(absolute)
		if seen[absolute] {
			continue
		}
		seen[absolute] = true
		out = append(out, absolute)
	}
	return out
}

func (m *Model) importLocalTreeEntry(
	ctx context.Context,
	source string,
	destination string,
	request uint32,
	skipped *[]string,
) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	info, err := os.Lstat(source)
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(source)
		if err != nil {
			return false, err
		}
		if !utf8.ValidString(target) {
			*skipped = append(*skipped, source+": symbolic-link target is not valid UTF-8")
			return false, nil
		}
		_, err = m.invoke(ctx, "fs_create_symlink", map[string]any{
			"path": destination, "target": filepath.ToSlash(target),
		})
		return err == nil, err
	}
	switch {
	case info.Mode().IsRegular():
		return m.importLocalFile(ctx, source, destination, request, info)
	case info.IsDir():
		entries, err := os.ReadDir(source)
		if err != nil {
			return false, err
		}
		if _, err := m.invoke(ctx, "fs_create_dir_exclusive", map[string]any{"path": destination}); err != nil {
			return false, err
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return true, err
			}
			name := entry.Name()
			if !utf8.ValidString(name) {
				*skipped = append(*skipped, filepath.Join(source, name)+": file name is not valid UTF-8")
				continue
			}
			childSource := filepath.Join(source, name)
			childDestination := path.Join(destination, filepath.ToSlash(name))
			if _, err := m.importLocalTreeEntry(ctx, childSource, childDestination, request, skipped); err != nil {
				return true, fmt.Errorf("%s: %w", childSource, err)
			}
		}
		_, err = m.invoke(ctx, "fs_set_permissions", map[string]any{
			"path": destination, "mode": remoteImportMode(info.Mode()),
		})
		return true, err
	default:
		*skipped = append(*skipped, source+": skipped special file")
		return false, nil
	}
}

func (m *Model) importLocalFile(
	ctx context.Context,
	source string,
	destination string,
	request uint32,
	expected os.FileInfo,
) (bool, error) {
	file, err := os.Open(source)
	if err != nil {
		return false, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return false, err
	}
	if !os.SameFile(expected, opened) {
		return false, fmt.Errorf("%s changed while the import was starting", source)
	}
	temporary, err := remoteImportTemporaryPath(destination)
	if err != nil {
		return false, err
	}
	if _, err := m.invoke(ctx, "fs_create_file", map[string]any{"path": temporary}); err != nil {
		return false, err
	}
	committed := false
	defer func() {
		if !committed {
			m.cleanupRemoteImportFile(temporary)
		}
	}()

	buffer := make([]byte, remoteImportChunkSize)
	for {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		count, readErr := file.Read(buffer)
		if count > 0 {
			if _, err := m.invoke(ctx, "fs_append_file_bytes", map[string]any{
				"path": temporary, "data": integerBytes(buffer[:count]), "request": request,
			}); err != nil {
				return false, err
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return false, readErr
		}
	}
	final, err := file.Stat()
	if err != nil {
		return false, err
	}
	if opened.Size() != final.Size() || !opened.ModTime().Equal(final.ModTime()) {
		return false, fmt.Errorf("%s changed while it was being imported", source)
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}

	// Publication is deliberately not tied to the user's cancel context. Once
	// the bytes are complete, cancellation must not leave an ambiguous rename
	// whose reply was dropped after the server committed it.
	commitCtx, cancelCommit := context.WithTimeout(m.remoteBaseContext(), 30*time.Second)
	defer cancelCommit()
	if _, err := m.invoke(commitCtx, "fs_set_permissions", map[string]any{
		"path": temporary, "mode": remoteImportMode(opened.Mode()),
	}); err != nil {
		return false, err
	}
	if _, err := m.invoke(commitCtx, "fs_rename", map[string]any{
		"from": temporary, "to": destination,
	}); err != nil {
		return false, err
	}
	committed = true
	return true, nil
}

func remoteImportTemporaryPath(destination string) (string, error) {
	random := make([]byte, 12)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return path.Join(path.Dir(destination), ".goid-import-"+hex.EncodeToString(random)+".part"), nil
}

func remoteImportMode(mode os.FileMode) uint32 {
	out := uint32(mode.Perm())
	if mode&os.ModeSetuid != 0 {
		out |= 0o4000
	}
	if mode&os.ModeSetgid != 0 {
		out |= 0o2000
	}
	if mode&os.ModeSticky != 0 {
		out |= 0o1000
	}
	return out
}

func (m *Model) cleanupRemoteImport(destination string) {
	if destination == "" {
		return
	}
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := m.invoke(cleanupCtx, "fs_remove_tree", map[string]any{"path": destination}); err == nil {
		return
	}
	// Older pinned servers lack fs_remove_tree. Keep a best-effort compatibility
	// cleanup so an attempted import does not leave the top-level entry behind.
	_, _ = m.invoke(cleanupCtx, "fs_delete", map[string]any{"path": destination})
}

func (m *Model) cleanupRemoteImportFile(destination string) {
	if destination == "" {
		return
	}
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := m.invoke(cleanupCtx, "fs_remove_file", map[string]any{"path": destination}); err == nil {
		return
	}
	_, _ = m.invoke(cleanupCtx, "fs_delete", map[string]any{"path": destination})
}

func integerBytes(data []byte) []int {
	out := make([]int, len(data))
	for index, value := range data {
		out[index] = int(value)
	}
	return out
}
