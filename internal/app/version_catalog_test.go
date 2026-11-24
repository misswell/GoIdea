package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui/editor"

	"mayoi/internal/docbridge"
	"mayoi/internal/openidea"
)

const versionCatalogTOML = `[versions]
springBoot = "3.5.16"
kotlin = "2.2.0" # comment

[libraries]
spring-boot-starter = { module = "org.springframework.boot:spring-boot-starter", version.ref = "springBoot" }
"jackson_kotlin" = "com.fasterxml.jackson.module:jackson-module-kotlin:2.19.0"
groovy.module = "org.apache.groovy:groovy"

[bundles]
web = [
  "spring-boot-starter",
  "jackson_kotlin",
]

[plugins]
kotlin-jvm = { id = "org.jetbrains.kotlin.jvm", version.ref = "kotlin" }
`

func TestGradleVersionCatalogParsingAndAccessors(t *testing.T) {
	catalog := parseGradleVersionCatalog(
		"libs",
		"/p/gradle/libs.versions.toml",
		versionCatalogTOML,
	)
	var got [][4]any
	for _, entry := range catalog.Entries {
		got = append(got, [4]any{entry.Section, entry.Alias, entry.Line, entry.Col})
	}
	want := [][4]any{
		{"versions", "springBoot", 1, 0},
		{"versions", "kotlin", 2, 0},
		{"libraries", "spring-boot-starter", 5, 0},
		{"libraries", "jackson_kotlin", 6, 1},
		{"libraries", "groovy", 7, 0},
		{"bundles", "web", 10, 0},
		{"plugins", "kotlin-jvm", 16, 0},
	}
	if len(got) != len(want) {
		t.Fatalf("entries = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("entry %d = %#v, want %#v", i, got[i], want[i])
		}
	}
	if catalog.Entries[1].Value != `"2.2.0"` {
		t.Fatalf("version value = %q", catalog.Entries[1].Value)
	}

	alias := func(segments ...string) string {
		entry, _ := gradleCatalogEntryFor(catalog, segments)
		return entry.Alias
	}
	if got := alias("spring", "boot", "starter"); got != "spring-boot-starter" {
		t.Fatalf("library alias = %q", got)
	}
	if got := alias("versions", "springBoot", "get"); got != "springBoot" {
		t.Fatalf("version alias = %q", got)
	}
	if got := alias("bundles", "web"); got != "web" {
		t.Fatalf("bundle alias = %q", got)
	}
	if got := alias("plugins", "kotlin", "jvm"); got != "kotlin-jvm" {
		t.Fatalf("plugin alias = %q", got)
	}
	if got := alias("spring", "web"); got != "" {
		t.Fatalf("unknown alias = %q", got)
	}
}

func TestGradleVersionCatalogBuildScriptChain(t *testing.T) {
	line := "    implementation(libs.spring.boot.starter)"
	chain, ok := gradleCatalogChainAt(line, strings.Index(line, "boot"))
	if !ok {
		t.Fatal("no accessor chain under the caret")
	}
	if chain.From != strings.Index(line, "libs") ||
		chain.To != strings.Index(line, ")") ||
		strings.Join(chain.Segments, ".") != "libs.spring.boot.starter" {
		t.Fatalf("chain = %+v", chain)
	}
	if _, ok := gradleCatalogChainAt(line, 6); ok {
		t.Fatal("a single word contributed a catalog chain")
	}
}

func TestGradleVersionCatalogAccessorCompletion(t *testing.T) {
	line := "dependencies { implementation(publishedLibs.kot"
	from, to, name, typed, ok := gradleCatalogCompletionAt(line, len(line))
	if !ok || name != "publishedLibs" || typed != "kot" ||
		line[from:to] != "publishedLibs.kot" {
		t.Fatalf(
			"completion context = from %d to %d name %q typed %q ok %v",
			from,
			to,
			name,
			typed,
			ok,
		)
	}
	download := &gradleVersionCatalogDownload{Catalogs: []gradleVersionCatalog{
		parseGradleVersionCatalog(
			"publishedLibs",
			"/cache/publishedLibs.toml",
			strings.Replace(
				versionCatalogTOML,
				"[libraries]\n",
				"[libraries]\nkotlin-stdlib = "+
					`"org.jetbrains.kotlin:kotlin-stdlib:2.2.0"`+"\n",
				1,
			),
		),
	}}
	items := gradleCatalogCompletionItems(
		download,
		name,
		typed,
		false,
	)
	if len(items) != 1 ||
		items[0].Insert != "publishedLibs.kotlin.stdlib" ||
		items[0].FilterText != "stdlib" {
		t.Fatalf("library completion = %+v", items)
	}

	bundleLine := "plugins { platform(publishedLibs.bundles.) }"
	_, _, name, typed, ok = gradleCatalogCompletionAt(
		bundleLine,
		strings.Index(bundleLine, "bundles.")+len("bundles."),
	)
	if !ok || name != "publishedLibs" || typed != "bundles." {
		t.Fatalf("bundle context = name %q typed %q ok %v", name, typed, ok)
	}
	items = gradleCatalogCompletionItems(download, name, typed, false)
	if len(items) != 1 || items[0].Insert != "publishedLibs.bundles.web" {
		t.Fatalf("bundle completion = %+v", items)
	}
}

func TestGradleVersionCatalogReadsDeclaredAndPublishedCatalogs(t *testing.T) {
	files := map[string]string{
		"/p/settings.gradle.kts": `dependencyResolutionManagement { versionCatalogs {
			create("platformLibs") { from("com.acme:platform-catalog:1.4.0") }
			create("later") { from("com.acme:later:1.0") }
		} }`,
		"/p/gradle/libs.versions.toml": versionCatalogTOML,
		"/cache/platform-catalog-1.4.0.toml": `[libraries]
spring-core = "org.springframework:spring-core:6.2.0"
`,
	}
	read := func(value string) (string, bool, error) {
		text, ok := files[value]
		return text, ok, nil
	}
	published := func(value string) (string, bool, error) {
		if value == "com.acme:platform-catalog:1.4.0" {
			return "/cache/platform-catalog-1.4.0.toml", true, nil
		}
		return "", false, nil
	}
	catalogs, complete, err := readGradleVersionCatalogs(
		"/p/build.gradle.kts",
		"/p",
		read,
		published,
	)
	if err != nil {
		t.Fatal(err)
	}
	if complete {
		t.Fatal("a catalog that is not downloaded yet reported complete")
	}
	if len(catalogs) != 2 {
		t.Fatalf("catalogs = %#v", catalogs)
	}
	byName := map[string]gradleVersionCatalog{}
	for _, catalog := range catalogs {
		byName[catalog.Name] = catalog
	}
	entry, ok := gradleCatalogEntryFor(
		byName["platformLibs"],
		[]string{"spring", "core"},
	)
	if !ok || entry.Value != `"org.springframework:spring-core:6.2.0"` {
		t.Fatalf("published entry = %+v, %v", entry, ok)
	}
	entry, ok = gradleCatalogEntryFor(
		byName["libs"],
		[]string{"spring", "boot", "starter"},
	)
	if !ok || entry.Alias != "spring-boot-starter" {
		t.Fatalf("default catalog entry = %+v, %v", entry, ok)
	}

	readErr := func(string) (string, bool, error) {
		return "", false, errors.New("backend unavailable")
	}
	if _, _, err := readGradleVersionCatalogs(
		"/p/build.gradle.kts",
		"/p",
		readErr,
		nil,
	); err == nil {
		t.Fatal("a backend read failure was ignored")
	}
}

func TestGradleVersionCatalogReferences(t *testing.T) {
	contents := `[versions]
springBoot = "3.5.16"
[libraries]
spring-boot-starter = { module = "org.springframework.boot:spring-boot-starter", version.ref = "springBoot" }
[bundles]
web = ["spring-boot-starter"]
`
	refOffset := strings.Index(contents, `"springBoot"`) + 2
	hit, ok := gradleCatalogReferenceAt(
		"libs",
		"/p/gradle/libs.versions.toml",
		contents,
		refOffset,
	)
	if !ok || hit.Entry.Section != "versions" || hit.Entry.Alias != "springBoot" {
		t.Fatalf("version.ref hit = %+v, %v", hit, ok)
	}
	if text := gradleCatalogHoverText(hit); !strings.Contains(text, "3.5.16") {
		t.Fatalf("hover text = %q", text)
	}

	bundleOffset := strings.Index(contents, `["spring-boot-starter"]`) + 3
	hit, ok = gradleCatalogReferenceAt(
		"libs",
		"/p/gradle/libs.versions.toml",
		contents,
		bundleOffset,
	)
	if !ok || hit.Entry.Section != "libraries" ||
		hit.Entry.Alias != "spring-boot-starter" {
		t.Fatalf("bundle hit = %+v, %v", hit, ok)
	}
	if _, ok := gradleCatalogReferenceAt(
		"libs",
		"/p/gradle/libs.versions.toml",
		contents,
		strings.Index(contents, `"org.springframework`)+2,
	); ok {
		t.Fatal("an ordinary library value contributed a reference")
	}
}

func TestGradleVersionCatalogSourceChangesInvalidateCache(t *testing.T) {
	model := &Model{}
	model.setRoot("/p")
	model.versionCatalogs.generation = 9
	model.versionCatalogs.downloads = map[string]*gradleVersionCatalogDownload{
		"/p/build.gradle.kts": {Ready: true},
	}
	model.acceptFsChanged(fsChangedForTest(
		"/p/gradle/libs.versions.toml",
	))
	if model.versionCatalogs.generation != 10 ||
		model.versionCatalogs.downloads != nil {
		t.Fatalf(
			"cache = generation %d downloads %#v",
			model.versionCatalogs.generation,
			model.versionCatalogs.downloads,
		)
	}
	if gradleCatalogSourceChanged(fsChangedForTest("/p/src/Main.java")) {
		t.Fatal("unrelated edit invalidated the cache")
	}
}

func fsChangedForTest(value string) openidea.FsChangedEvent {
	return openidea.FsChangedEvent{Modified: []string{value}}
}

func gradleCatalogAcceptanceBinary(t *testing.T) string {
	t.Helper()
	if binary := strings.TrimSpace(os.Getenv("GOIDEA_GRADLE")); binary != "" {
		if _, err := os.Stat(binary); err != nil {
			t.Fatalf("GOIDEA_GRADLE: %v", err)
		}
		return binary
	}
	if home := strings.TrimSpace(os.Getenv("GOIDEA_GRADLE_HOME")); home != "" {
		binary := filepath.Join(home, "bin", "gradle")
		if _, err := os.Stat(binary); err != nil {
			t.Fatalf("GOIDEA_GRADLE_HOME: %v", err)
		}
		return binary
	}
	binary, err := exec.LookPath("gradle")
	if err != nil {
		t.Skip("set GOIDEA_GRADLE_HOME to the Gradle 8.10.2 installation")
	}
	return binary
}

func gradleCatalogEnvironment(overrides map[string]string) []string {
	remove := map[string]bool{
		"GOIDEA_CATALOG_REPOSITORY": true,
		"GRADLE_USER_HOME":          true,
	}
	out := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if !remove[key] {
			out = append(out, entry)
		}
	}
	for key, value := range overrides {
		out = append(out, key+"="+value)
	}
	return out
}

func runGradleCatalogAcceptance(
	t *testing.T,
	binary, dir, gradleHome string,
	environment map[string]string,
	args ...string,
) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	commandArgs := []string{
		"--offline",
		"--no-daemon",
		"--console=plain",
		"--gradle-user-home", gradleHome,
	}
	commandArgs = append(commandArgs, args...)
	cmd := exec.CommandContext(ctx, binary, commandArgs...)
	cmd.Dir = dir
	cmd.Env = gradleCatalogEnvironment(environment)
	raw, err := cmd.CombinedOutput()
	output := string(raw)
	if err != nil {
		if len(output) > 12000 {
			output = output[len(output)-12000:]
		}
		t.Fatalf("Gradle %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return output
}

func copyGradleCatalogFixture(
	t *testing.T,
	source, destination string,
) {
	t.Helper()
	err := filepath.Walk(source, func(
		path string,
		info os.FileInfo,
		walkErr error,
	) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, raw, info.Mode().Perm())
	})
	if err != nil {
		t.Fatalf("copy Gradle catalog fixture: %v", err)
	}
}

func loadGradleCatalogForAcceptance(
	t *testing.T,
	model *Model,
	script string,
) *gradleVersionCatalogDownload {
	t.Helper()
	done := make(chan struct{}, 1)
	model.postHook = func(fn func()) {
		fn()
		done <- struct{}{}
	}
	download := model.gradleVersionCatalogDownload(script, "publishedLibs")
	if download == nil {
		t.Fatal("catalog load did not start")
	}
	if !download.Ready {
		select {
		case <-done:
		case <-time.After(2 * time.Minute):
			t.Fatal("catalog load did not finish")
		}
	}
	download = model.versionCatalogs.downloads[script]
	if download == nil || !download.Ready || download.Err != "" {
		t.Fatalf("catalog download = %+v", download)
	}
	return download
}

func TestIntegrationGradlePublishedVersionCatalog(t *testing.T) {
	gradleBinary := gradleCatalogAcceptanceBinary(t)
	versionContext, versionCancel := context.WithTimeout(
		context.Background(),
		time.Minute,
	)
	defer versionCancel()
	versionCommand := exec.CommandContext(versionContext, gradleBinary, "--version")
	versionOutput, err := versionCommand.CombinedOutput()
	if err != nil {
		t.Fatalf("Gradle --version: %v\n%s", err, versionOutput)
	}
	versionText := string(versionOutput)
	if !strings.Contains(versionText, "Gradle 8.10.2") {
		t.Fatalf("the fixed acceptance requires Gradle 8.10.2:\n%s", versionText)
	}
	t.Log(strings.SplitN(versionText, "\n", 2)[0])

	manager, root := launchParityIntegrationServer(t)
	_, sourceFile, _, _ := runtime.Caller(0)
	fixtureRoot := filepath.Join(
		filepath.Dir(sourceFile),
		"..",
		"..",
		"testdata",
		"gradle-version-catalog",
	)
	publisher := filepath.Join(root, "publisher")
	consumer := filepath.Join(root, "consumer")
	copyGradleCatalogFixture(
		t,
		filepath.Join(fixtureRoot, "publisher"),
		publisher,
	)
	copyGradleCatalogFixture(
		t,
		filepath.Join(fixtureRoot, "consumer"),
		consumer,
	)
	repository := filepath.Join(root, "local-maven")
	gradleHome, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repositoryURI := "file://" + filepath.ToSlash(repository)
	publishOutput := runGradleCatalogAcceptance(
		t,
		gradleBinary,
		publisher,
		gradleHome,
		map[string]string{"GRADLE_USER_HOME": gradleHome},
		"-PacceptanceRepository="+repositoryURI,
		"publishAllPublicationsToAcceptanceRepository",
	)
	if !strings.Contains(publishOutput, "BUILD SUCCESSFUL") {
		t.Fatalf("Gradle publication output:\n%s", publishOutput)
	}
	publishedTOML := filepath.Join(
		repository,
		"dev",
		"goidea",
		"platform-catalog",
		"1.2.3",
		"platform-catalog-1.2.3.toml",
	)
	if info, err := os.Stat(publishedTOML); err != nil || info.IsDir() {
		t.Fatalf("Gradle did not publish the catalog TOML: %v", err)
	}
	consumeOutput := runGradleCatalogAcceptance(
		t,
		gradleBinary,
		consumer,
		gradleHome,
		map[string]string{
			"GRADLE_USER_HOME":          gradleHome,
			"GOIDEA_CATALOG_REPOSITORY": repositoryURI,
		},
		"verifyCatalog",
	)
	for _, marker := range []string{
		"goidea-catalog-version=2.2.0",
		"goidea-catalog-library=org.jetbrains.kotlin:kotlin-stdlib:2.2.0",
		"goidea-catalog-bundle-size=2",
	} {
		if !strings.Contains(consumeOutput, marker) {
			t.Fatalf("Gradle consumer did not print %q:\n%s", marker, consumeOutput)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	encodedHome, err := json.Marshal(gradleHome)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Invoke(ctx, Window, "settings_set", map[string]any{
		"scope": "project",
		"key":   "languages.java.gradle.userHome",
		"value": string(encodedHome),
	}); err != nil {
		t.Fatalf("set isolated Gradle user home: %v", err)
	}
	encodedRepository, err := json.Marshal(repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Invoke(ctx, Window, "settings_set", map[string]any{
		"scope": "project",
		"key":   "languages.java.maven.localRepository",
		"value": string(encodedRepository),
	}); err != nil {
		t.Fatalf("set acceptance repository: %v", err)
	}

	coordinates := "dev.goidea:platform-catalog:1.2.3"
	rawPath, err := manager.Invoke(ctx, Window, "gradle_published_catalog", map[string]any{
		"coordinates": coordinates,
	})
	if err != nil {
		t.Fatalf("gradle_published_catalog: %v", err)
	}
	var catalogPath string
	if err := json.Unmarshal(rawPath, &catalogPath); err != nil {
		t.Fatalf("gradle_published_catalog reply %s: %v", rawPath, err)
	}
	wantCacheRoot := filepath.Join(
		gradleHome,
		"caches",
		"modules-2",
		"files-2.1",
	)
	wantRepository := filepath.Join(
		repository,
		"dev",
		"goidea",
		"platform-catalog",
		"1.2.3",
	)
	if !gradleCatalogContains(
		filepath.ToSlash(wantCacheRoot),
		filepath.ToSlash(catalogPath),
	) && !gradleCatalogContains(
		filepath.ToSlash(wantRepository),
		filepath.ToSlash(catalogPath),
	) {
		t.Fatalf("catalog path %q is outside the isolated Gradle and acceptance repository caches", catalogPath)
	}

	read := func(value string) (string, bool, error) {
		raw, err := manager.Invoke(ctx, Window, "fs_read_text", map[string]any{
			"path": value,
		})
		if err != nil {
			if openidea.ErrorKind(err) == "notFound" {
				return "", false, nil
			}
			return "", false, err
		}
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return "", false, err
		}
		return text, true, nil
	}
	published := func(value string) (string, bool, error) {
		if value != coordinates {
			return "", false, nil
		}
		raw, err := manager.Invoke(ctx, Window, "gradle_published_catalog", map[string]any{
			"coordinates": value,
		})
		if err != nil {
			if openidea.ErrorKind(err) == "notFound" {
				return "", false, nil
			}
			return "", false, err
		}
		var path string
		if err := json.Unmarshal(raw, &path); err != nil {
			return "", false, err
		}
		return path, path != "", nil
	}
	consumerBuild := filepath.Join(consumer, "build.gradle")
	catalogs, _, err := readGradleVersionCatalogs(
		consumerBuild,
		root,
		read,
		published,
	)
	if err != nil {
		t.Fatalf("read catalogs through the real RPC: %v", err)
	}
	if len(catalogs) != 1 || catalogs[0].Name != "publishedLibs" ||
		catalogs[0].Path != catalogPath {
		t.Fatalf("catalogs = %+v, path = %q", catalogs, catalogPath)
	}
	catalogText, ok, err := read(catalogPath)
	if err != nil || !ok {
		t.Fatalf("read published catalog: ok=%v err=%v", ok, err)
	}

	model := &Model{ctx: ctx, mgr: manager}
	model.setRoot(root)
	download := loadGradleCatalogForAcceptance(t, model, consumerBuild)
	if len(download.Catalogs) != 1 {
		t.Fatalf("backend-loaded catalogs = %+v", download.Catalogs)
	}
	accessor, ok := gradleCatalogHitForChain(
		download,
		[]string{"publishedLibs", "kotlin", "stdlib"},
	)
	if !ok || accessor.Entry.Alias != "kotlin-stdlib" ||
		!strings.Contains(gradleCatalogHoverText(accessor), "2.2.0") {
		t.Fatalf("library accessor = %+v, %v", accessor, ok)
	}
	versionRefOffset := strings.Index(catalogText, `ref = "kotlin"`) +
		len(`ref = "`) + 2
	versionRef, ok := gradleCatalogReferenceAt(
		"publishedLibs",
		catalogPath,
		catalogText,
		versionRefOffset,
	)
	if !ok || versionRef.Entry.Section != "versions" ||
		versionRef.Entry.Alias != "kotlin" {
		t.Fatalf("version.ref navigation = %+v, %v", versionRef, ok)
	}
	bundleSection := strings.Index(catalogText, "[bundles]")
	if bundleSection < 0 {
		t.Fatalf("published catalog has no bundle: %s", catalogText)
	}
	bundleOffset := bundleSection +
		strings.Index(catalogText[bundleSection:], `"kotlin-stdlib"`) +
		3
	bundle, ok := gradleCatalogReferenceAt(
		"publishedLibs",
		catalogPath,
		catalogText,
		bundleOffset,
	)
	if !ok || bundle.Entry.Section != "libraries" ||
		bundle.Entry.Alias != "kotlin-stdlib" {
		t.Fatalf("bundle navigation = %+v, %v", bundle, ok)
	}

	buildText := "dependencies {\n" +
		"  implementation(publishedLibs.kotlin.stdlib)\n" +
		"  implementation(publishedLibs.kotlin.st)\n" +
		"}\n"
	buildDoc := docbridge.Virtual(consumerBuild, buildText)
	buildTab := &Tab{
		doc:   buildDoc,
		state: &editor.State{Buffer: buildDoc.Buffer()},
	}
	catalogDoc := docbridge.Virtual(catalogPath, catalogText)
	catalogTab := &Tab{
		doc:   catalogDoc,
		state: &editor.State{Buffer: catalogDoc.Buffer()},
	}
	model.tabs = []*Tab{buildTab, catalogTab}
	model.active = 0

	completionOffset := strings.LastIndex(
		buildText,
		"publishedLibs.kotlin.st",
	) + len("publishedLibs.kotlin.st")
	buildTab.state.Select(completionOffset, completionOffset)
	model.requestCompletion(buildTab, editor.CompletionRequest{
		Offset:   completionOffset,
		Prefix:   "st",
		Version:  buildDoc.Buffer().Version(),
		Explicit: true,
	})
	if !buildTab.completion.open || len(buildTab.completion.infos) == 0 {
		t.Fatalf(
			"native catalog completion = open %v infos %d",
			buildTab.completion.open,
			len(buildTab.completion.infos),
		)
	}

	accessorOffset := strings.Index(buildText, "publishedLibs.kotlin.stdlib") +
		len("publishedLibs.kotlin.st")
	model.gotoDefinition(buildTab, accessorOffset)
	if model.tab() != catalogTab || catalogTab.state.SelectedText() != "kotlin-stdlib" {
		t.Fatalf(
			"libs accessor opened %+v with selection %q",
			model.tab(),
			catalogTab.state.SelectedText(),
		)
	}
	model.gotoDefinition(catalogTab, versionRefOffset)
	if catalogTab.state.SelectedText() != "kotlin" {
		t.Fatalf("version.ref selected %q", catalogTab.state.SelectedText())
	}
	model.gotoDefinition(catalogTab, bundleOffset)
	if catalogTab.state.SelectedText() != "kotlin-stdlib" {
		t.Fatalf("bundle member selected %q", catalogTab.state.SelectedText())
	}

	rpcCalls := 0
	model.invokeHook = func(
		ctx context.Context,
		command string,
		args any,
	) (json.RawMessage, error) {
		if command == "gradle_published_catalog" {
			rpcCalls++
		}
		return manager.Invoke(ctx, Window, command, args)
	}
	// Drop the pre-hook load so this measures a cold resolved-catalog fetch
	// and the following settings invalidation independently.
	model.versionCatalogs.downloads = nil
	loadGradleCatalogForAcceptance(t, model, consumerBuild)
	if rpcCalls != 1 {
		t.Fatalf("initial catalog load used %d publication RPCs", rpcCalls)
	}
	model.acceptFsChanged(fsChangedForTest(
		filepath.Join(consumer, "settings.gradle"),
	))
	if model.versionCatalogs.downloads != nil {
		t.Fatal("settings change did not invalidate the published catalog cache")
	}
	reloaded := loadGradleCatalogForAcceptance(t, model, consumerBuild)
	if rpcCalls != 2 || len(reloaded.Catalogs) != 1 {
		t.Fatalf(
			"cache reload used %d publication RPCs with %+v",
			rpcCalls,
			reloaded.Catalogs,
		)
	}
	recordParityIntegrationEvidence(t, "gradleVersionCatalogPublication")
}
