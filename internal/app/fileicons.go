package app

// OpenIDEA's file and folder icons, ported from src/ui/icons.tsx: exact file
// names, then name patterns, then the two-part extension, then the extension.
// The result is the vendored asset path, which oiIconEl draws in the icon's
// own palette (file types are coloured; folders are monochrome).

import (
	"path"
	"regexp"
	"strings"

	"github.com/egoist/mygo/ui"
)

// fileIconPaths maps the icon names of fileIconName's tables to the vendored
// drawings, as the ICONS table of icons.tsx does.
var fileIconPaths = map[string]string{
	"file-unknown":       "fileTypes/unknown",
	"file-code":          "fileTypes/anyType",
	"file-text":          "fileTypes/text",
	"file-json":          "fileTypes/json",
	"file-md":            "fileTypes/markdown",
	"file-image":         "fileTypes/image",
	"file-config":        "fileTypes/config",
	"file-python":        "fileTypes/python",
	"file-pyproject":     "fileTypes/pyprojectToml",
	"file-pipenv":        "fileTypes/pipenv",
	"file-requirements":  "fileTypes/requirements",
	"file-jupyter":       "fileTypes/jupyter",
	"file-js":            "fileTypes/javaScript",
	"file-jsx":           "openidea/fileTypes/jsx",
	"file-ts":            "fileTypes/typeScript",
	"file-tsx":           "openidea/fileTypes/tsx",
	"file-npm":           "openidea/fileTypes/npm",
	"file-vue":           "fileTypes/vue",
	"file-svelte":        "openidea/fileTypes/svelte",
	"file-jsp":           "openidea/fileTypes/jsp",
	"file-astro":         "fileTypes/astro",
	"file-yaml":          "fileTypes/yaml",
	"file-toml":          "fileTypes/toml",
	"file-html":          "fileTypes/html",
	"file-xhtml":         "fileTypes/xhtml",
	"file-css":           "fileTypes/css",
	"file-less":          "openidea/fileTypes/less",
	"file-sass":          "openidea/fileTypes/sass",
	"file-scss":          "openidea/fileTypes/scss",
	"file-xml":           "fileTypes/xml",
	"file-xsd":           "fileTypes/xsd",
	"file-shell":         "fileTypes/shell",
	"file-powershell":    "openidea/fileTypes/powershell",
	"file-windows":       "fileTypes/microsoftWindows",
	"file-sql":           "fileTypes/sql",
	"file-graphql":       "fileTypes/graphql",
	"file-proto":         "fileTypes/protoFile",
	"file-csv":           "fileTypes/csv",
	"file-properties":    "fileTypes/properties",
	"file-ignored":       "fileTypes/ignored",
	"file-git":           "fileTypes/gitignore",
	"file-archive":       "fileTypes/archive",
	"file-binary":        "fileTypes/binaryData",
	"file-font":          "fileTypes/font",
	"file-patch":         "fileTypes/patch",
	"file-java":          "fileTypes/java",
	"file-kotlin":        "fileTypes/kotlin",
	"file-kotlin-script": "fileTypes/kotlinScript",
	"file-groovy":        "fileTypes/groovy",
	"file-gradle":        "fileTypes/gradle",
	"file-gradle-kts":    "fileTypes/kotlinGradleScript",
	"file-maven":         "fileTypes/maven",
	"file-scala":         "language/scala",
	"file-php":           "language/php",
	"file-perl":          "fileTypes/perl",
	"file-c":             "fileTypes/c",
	"file-cpp":           "fileTypes/cpp",
	"file-h":             "fileTypes/h",
	"file-objc":          "openidea/fileTypes/objectiveC",
	"file-objcpp":        "openidea/fileTypes/objectiveCpp",
	"file-csharp":        "fileTypes/Csharp",
	"file-fsharp":        "openidea/fileTypes/fsharp",
	"file-swift":         "fileTypes/swiftLang",
	"file-go":            "openidea/fileTypes/go",
	"file-gomod":         "openidea/fileTypes/goMod",
	"file-gowork":        "openidea/fileTypes/goWork",
	"file-rust":          "fileTypes/rust",
	"file-cargo":         "fileTypes/cargo",
	"file-cargo-lock":    "fileTypes/cargoLock",
	"file-ruby":          "openidea/fileTypes/ruby",
	"file-lua":           "openidea/fileTypes/lua",
	"file-zig":           "openidea/fileTypes/zig",
	"file-gdscript":      "openidea/fileTypes/gdscript",
	"file-godot":         "openidea/fileTypes/godot",
	"file-gdshader":      "openidea/fileTypes/gdshader",
	"file-asciidoc":      "openidea/fileTypes/asciidoc",
	"file-d":             "openidea/fileTypes/d",
	"file-prisma":        "openidea/fileTypes/prisma",
	"file-dart":          "openidea/fileTypes/dart",
	"file-elixir":        "openidea/fileTypes/elixir",
	"file-erlang":        "openidea/fileTypes/erlang",
	"file-haskell":       "openidea/fileTypes/haskell",
	"file-ocaml":         "openidea/fileTypes/ocaml",
	"file-clojure":       "openidea/fileTypes/clojure",
	"file-julia":         "openidea/fileTypes/julia",
	"file-r":             "openidea/fileTypes/r",
	"file-nix":           "openidea/fileTypes/nix",
	"file-latex":         "openidea/fileTypes/latex",
	"file-rst":           "fileTypes/rst",
	"file-docker":        "fileTypes/docker",
	"file-makefile":      "fileTypes/makefile",
	"file-cmake":         "openidea/fileTypes/cmake",
	"file-bazel":         "fileTypes/bazel",
	"file-jenkins":       "fileTypes/jenkins",
	"file-terraform":     "fileTypes/terraform",
	"file-hcl":           "fileTypes/hashiCorp",
	"file-dotenv":        "openidea/fileTypes/dotenv",
	"file-lock":          "openidea/fileTypes/lock",
	"file-license":       "openidea/fileTypes/license",
	"file-readme":        "openidea/fileTypes/readme",
	"file-log":           "openidea/fileTypes/log",
	"file-http":          "fileTypes/http",
	"file-regexp":        "fileTypes/regexp",
	"file-manifest":      "fileTypes/manifest",
	"file-idl":           "fileTypes/idl",
}

// folderIconPaths maps folderIconName's kinds to the vendored drawings.
var folderIconPaths = map[string]string{
	"plain":           "nodes/folder",
	"source":          "nodes/sourceRoot",
	"test":            "nodes/testRoot",
	"resources":       "nodes/resourcesRoot",
	"testResources":   "openidea/nodes/testResourcesRoot",
	"excluded":        "nodes/excludeRoot",
	"library":         "nodes/libraryFolder",
	"github":          "nodes/folderGithub",
	"module":          "nodes/module",
	"package":         "nodes/package",
	"generatedSource": "nodes/generatedSource",
	"generatedTest":   "nodes/generatedTestRoot",
	"compiled":        "nodes/compiledClassesFolder",
	"javadoc":         "nodes/javaDocFolder",
}

// fileIconTable turns `icon: "a b c"` groups into one map, as icons.tsx does.
func fileIconTable(groups map[string]string) map[string]string {
	out := map[string]string{}
	for icon, keys := range groups {
		for _, key := range strings.Fields(keys) {
			out[key] = icon
		}
	}
	return out
}

// fileIconNames is FILE_NAMES: lower-case file names with their icon.
var fileIconNames = fileIconTable(map[string]string{
	"file-docker":     "dockerfile containerfile",
	"file-makefile":   "makefile gnumakefile bsdmakefile makefile.am makefile.in kbuild",
	"file-cmake":      "cmakelists.txt cmakecache.txt",
	"file-perl":       "makefile.pl build.pl cpanfile",
	"file-git":        ".gitattributes .gitmodules .gitconfig .git-blame-ignore-revs .mailmap",
	"file-ignored":    ".gitignore .gitignore_global .dockerignore .npmignore .prettierignore .eslintignore .stylelintignore .hgignore .ignore .rgignore .fdignore .helmignore .vscodeignore .gcloudignore .slugignore .cfignore .bzrignore .containerignore",
	"file-config":     ".editorconfig justfile procfile .npmrc .nvmrc .yarnrc .node-version .python-version .ruby-version .tool-versions .browserslistrc .pypirc .flake8 .pylintrc pylintrc setup.cfg tox.ini pytest.ini mypy.ini .coveragerc .hgrc hgrc credentials .jshintrc .gemrc .clang-format .clang-tidy .clangd .watchmanconfig .arcconfig",
	"file-npm":        "package.json",
	"file-lock":       "package-lock.json npm-shrinkwrap.json pnpm-lock.yaml bun.lockb deno.lock go.sum go.work.sum package.resolved",
	"file-cargo":      "cargo.toml",
	"file-cargo-lock": "cargo.lock",
	"file-gomod":      "go.mod",
	"file-gowork":     "go.work",
	"file-pyproject":  "pyproject.toml",
	"file-pipenv":     "pipfile",
	"file-python":     "sconstruct sconscript",
	"file-ruby":       "gemfile rakefile podfile vagrantfile brewfile guardfile fastfile appfile capfile dangerfile steepfile .irbrc .pryrc",
	"file-shell":      ".bashrc .bash_profile .bash_login .bash_logout .bash_aliases .profile .zshrc .zshenv .zprofile .zlogin .zlogout .kshrc pkgbuild apkbuild .envrc",
	"file-dotenv":     ".flaskenv",
	"file-gradle":     "gradlew",
	"file-maven":      "pom.xml mvnw",
	"file-jenkins":    "jenkinsfile",
	"file-bazel":      "build workspace .bazelrc .bazelversion",
	"file-terraform":  ".terraformrc terraform.rc",
	"file-clojure":    "deps.edn project.clj bb.edn",
	"file-erlang":     "rebar.config sys.config",
	"file-r":          ".rprofile",
	"file-lua":        ".luacheckrc",
	"file-yaml":       "citation.cff",
	"file-manifest":   "manifest.mf",
	"file-readme":     "readme readme.txt",
	"file-text":       "authors notice codeowners contributors changelog",
})

// fileIconPatterns is NAME_PATTERNS, tried in order.
var fileIconPatterns = []struct {
	re   *regexp.Regexp
	icon string
}{
	{regexp.MustCompile(`^(dockerfile|containerfile)[.-]`), "file-docker"},
	{regexp.MustCompile(`^.*compose.*\.ya?ml$`), "file-docker"},
	{regexp.MustCompile(`^\.env(\.|$)`), "file-dotenv"},
	{regexp.MustCompile(`^(licen[cs]e|copying)(-[\w.-]+|\.(md|txt|rst))?$`), "file-license"},
	{regexp.MustCompile(`^requirements([.-][\w.-]*)?\.(txt|in)$`), "file-requirements"},
	{regexp.MustCompile(`^(ts|js)config(\.[\w-]+)?\.json$`), "file-config"},
	{regexp.MustCompile(`\.config\.[cm]?[jt]s$`), "file-config"},
	{regexp.MustCompile(`^\.(eslintrc|prettierrc|stylelintrc|babelrc|swcrc|lintstagedrc|commitlintrc|markdownlintrc)(\.|$)`), "file-config"},
}

// fileIconExts is EXT_ICONS, including the two-part "gradle.kts".
var fileIconExts = fileIconTable(map[string]string{
	"file-python":        "py pyi pyw pyx pxd",
	"file-jupyter":       "ipynb",
	"file-js":            "js mjs cjs",
	"file-jsx":           "jsx",
	"file-ts":            "ts mts cts",
	"file-tsx":           "tsx",
	"file-vue":           "vue",
	"file-svelte":        "svelte",
	"file-jsp":           "jsp jspx jspf jsph tag tagf tagx",
	"file-astro":         "astro",
	"file-json":          "json jsonc json5 jsonl ndjson geojson topojson webmanifest har avsc code-workspace code-snippets map",
	"file-md":            "md mdx markdown mdown mkd mkdn mdwn ronn workbook",
	"file-rst":           "rst",
	"file-yaml":          "yaml yml",
	"file-toml":          "toml",
	"file-text":          "txt text",
	"file-log":           "log",
	"file-csv":           "csv tsv",
	"file-html":          "html htm shtml html.tmpl djhtml",
	"file-xhtml":         "xhtml",
	"file-css":           "css pcss postcss",
	"file-less":          "less",
	"file-sass":          "sass",
	"file-scss":          "scss",
	"file-xml":           "xml xsl xslt plist csproj fsproj vbproj props targets nuspec xaml axaml resx wsdl fxml iml jnlp rss atom xliff xlf storyboard xib tld pom",
	"file-xsd":           "xsd",
	"file-shell":         "sh bash zsh ksh ash dash bats fish command",
	"file-powershell":    "ps1 psm1 psd1 pssc psrc",
	"file-windows":       "bat cmd exe",
	"file-sql":           "sql ddl dml psql pgsql mysql",
	"file-graphql":       "graphql graphqls gql",
	"file-proto":         "proto",
	"file-image":         "png jpg jpeg gif svg webp ico icns bmp tif tiff avif",
	"file-archive":       "zip tar gz tgz bz2 xz zst 7z rar jar war whl egg",
	"file-config":        "ini cfg conf cnf inf reg url desktop service socket timer config",
	"file-dotenv":        "env",
	"file-lock":          "lock lockb",
	"file-ignored":       "dockerignore gitignore",
	"file-properties":    "properties prefs",
	"file-font":          "ttf otf woff woff2",
	"file-patch":         "patch diff rej",
	"file-java":          "java",
	"file-kotlin":        "kt",
	"file-kotlin-script": "kts",
	"file-groovy":        "groovy gvy gy gsh",
	"file-gradle":        "gradle",
	"file-gradle-kts":    "gradle.kts",
	"file-scala":         "scala sc sbt mill",
	"file-php":           "php phtml php3 php4 php5 phps phpt",
	"file-perl":          "pl pm pod psgi cgi",
	"file-c":             "c",
	"file-h":             "h hpp hh hxx h++",
	"file-cpp":           "cpp cc cxx c++ ipp tpp inl ixx cppm ino",
	"file-objc":          "m",
	"file-objcpp":        "mm",
	"file-csharp":        "cs csx",
	"file-fsharp":        "fs fsi fsx",
	"file-swift":         "swift swiftinterface",
	"file-go":            "go",
	"file-rust":          "rs",
	"file-ruby":          "rb rake gemspec ru rbi jbuilder podspec erb rhtml",
	"file-lua":           "lua luau rockspec",
	"file-zig":           "zig zon",
	"file-gdscript":      "gd",
	"file-godot":         "tscn tres godot import",
	"file-gdshader":      "gdshader gdshaderinc",
	"file-asciidoc":      "adoc asciidoc",
	"file-d":             "d di",
	"file-prisma":        "prisma",
	"file-dart":          "dart",
	"file-elixir":        "ex exs",
	"file-erlang":        "erl hrl escript app.src",
	"file-haskell":       "hs lhs hs-boot hsig",
	"file-ocaml":         "ml mli mll mly",
	"file-clojure":       "clj cljs cljc cljd edn bb",
	"file-julia":         "jl",
	"file-r":             "r rprofile",
	"file-nix":           "nix",
	"file-latex":         "tex ltx sty cls dtx ins bbx cbx bib",
	"file-docker":        "dockerfile containerfile",
	"file-makefile":      "mk mak make",
	"file-cmake":         "cmake",
	"file-bazel":         "bazel bzl",
	"file-terraform":     "tf tofu tfvars tftest.hcl",
	"file-hcl":           "hcl nomad pkr.hcl",
	"file-http":          "http rest",
	"file-regexp":        "regexp",
	"file-idl":           "idl",
	"file-binary":        "class bin dat so dylib dll o a wasm pyc",
})

// fileCodeExts are source extensions drawn with the generic code icon.
var fileCodeExts = map[string]bool{"nim": true, "wit": true, "wat": true, "sol": true, "v": true, "sv": true}

// fileIconName picks the icon name for a file, in OpenIDEA's lookup order.
func fileIconName(fileName string) string {
	lower := strings.ToLower(path.Base(fileName))
	if named, ok := fileIconNames[lower]; ok {
		return named
	}
	for _, p := range fileIconPatterns {
		if p.re.MatchString(lower) {
			return p.icon
		}
	}
	dot := strings.LastIndex(lower, ".")
	if dot <= 0 {
		return "file-text"
	}
	ext := lower[dot+1:]
	prev := strings.LastIndex(lower[:dot], ".")
	ext2 := ""
	if prev > 0 {
		ext2 = lower[prev+1:]
	}
	if ext2 != "" {
		if icon, ok := fileIconExts[ext2]; ok {
			return icon
		}
	}
	if icon, ok := fileIconExts[ext]; ok {
		return icon
	}
	if fileCodeExts[ext] {
		return "file-code"
	}
	return "file-unknown"
}

// fileIconAsset is the vendored drawing of a file name.
func fileIconAsset(fileName string) string {
	if p, ok := fileIconPaths[fileIconName(fileName)]; ok {
		return p
	}
	return "fileTypes/unknown"
}

// folderIconAsset is the vendored drawing of a folder kind.
func folderIconAsset(kind string) string {
	if p, ok := folderIconPaths[kind]; ok {
		return p
	}
	return "nodes/folder"
}

// fileIconEl draws a file's icon: file types keep their palette, so the tone
// only reaches the monochrome fallbacks.
func fileIconEl(c *ui.Context, oi oiColors, fileName string, size float32, tint ui.Color) ui.Element {
	return oiIconEl(c, fileIconAsset(fileName), size, oi, tint)
}

// folderIconEl draws a folder's icon of the given role.
func folderIconEl(c *ui.Context, oi oiColors, kind string, size float32, tint ui.Color) ui.Element {
	return oiIconEl(c, folderIconAsset(kind), size, oi, tint)
}
