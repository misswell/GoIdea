package app

import (
	"testing"

	"mayoi/internal/openidea"
)

func remoteTreeFixture(t *testing.T) (*Model, string, string) {
	t.Helper()
	root := "/srv/app"
	file := root + "/src/Main.java"
	tree := NewRemoteTree(root, func(_ string, done func([]openidea.RemoteDirectoryEntry, error)) {
		done(nil, nil)
	}, func(fn func()) { fn() }, func() {})
	tree.remoteEntries[root] = []openidea.RemoteDirectoryEntry{
		{Name: "src", Path: root + "/src", IsDir: true},
	}
	tree.remoteEntries[root+"/src"] = []openidea.RemoteDirectoryEntry{
		{Name: "Main.java", Path: file},
	}
	tree.open[root+"/src"] = true
	tree.rebuildRemote()
	m := &Model{
		remote: remoteDevelopmentState{
			session: &openidea.RemoteSession{
				Target: openidea.RemoteTarget{Kind: openidea.RemoteTargetSSH},
				Root:   root,
				OS:     "linux",
			},
		},
	}
	m.setRoot(root)
	m.tree = tree
	t.Cleanup(tree.Close)
	return m, root, file
}

func TestRemoteRenameUsesTheKnownTreeEntryInsteadOfLocalStat(t *testing.T) {
	m, _, file := remoteTreeFixture(t)
	m.OpenFileRenameDialog(file)
	if m.fileRefactor == nil {
		t.Fatal("remote Rename dialog did not open")
	}
	if m.fileRefactor.kind != "rename" || m.fileRefactor.path != file || m.fileRefactor.dir {
		t.Fatalf("remote Rename state = %+v", m.fileRefactor)
	}
}

func TestRemoteProjectTreeRowsAreDraggableWithoutLocalFiles(t *testing.T) {
	m, _, file := remoteTreeFixture(t)
	got := m.draggableTreePaths(file)
	if len(got) != 1 || got[0] != file {
		t.Fatalf("draggable remote paths = %v, want %s", got, file)
	}
}

func TestRemoteServerRootRejectsNonPosixAndNetworkPaths(t *testing.T) {
	for _, path := range []string{"/srv/app", "/Users/dev/project"} {
		if !validRemoteServerPath(path) {
			t.Fatalf("valid remote path rejected: %s", path)
		}
	}
	for _, path := range []string{
		"", "relative/path", "C:/work/app", `C:\work\app`,
		"//server/share", `\\server\share`, "//./pipe/devcontainer",
	} {
		if validRemoteServerPath(path) {
			t.Fatalf("invalid remote path accepted: %q", path)
		}
	}
}
