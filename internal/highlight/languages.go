package highlight

import (
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/egoist/mygo/ui/editor"
)

// Language describes OpenIDEA's language contribution in the form MyGo needs
// to detect a file and edit it. ID and Name follow languages/*.toml. Core is
// the syntax family used when Chroma has no closer renderer for the exact
// contribution. Metadata marks comment and syntax support that the native
// editor can apply without pretending to implement a missing parser.
type Language struct {
	ID              string
	Name            string
	Extensions      []string
	Filenames       []string
	UIMode          string
	Core            string
	LineComment     string
	BlockComment    [2]string
	TabSize         int
	InsertSpaces    bool
	TreeSitter      string
	LanguageServers []string
	Debugger        bool
}

type languageDefinition struct {
	Language
}

var languageCatalog = []Language{
	{ID: "arduino", Name: "Arduino", Extensions: []string{"ino"}, UIMode: "C++", Core: "cpp", LineComment: "//", BlockComment: [2]string{"/*", "*/"}, TabSize: 2, InsertSpaces: true, TreeSitter: "cpp", LanguageServers: []string{"arduino-language-server"}},
	{ID: "asciidoc", Name: "AsciiDoc", Extensions: []string{"adoc", "asciidoc"}, UIMode: "AsciiDoc", Core: "asciidoc", LineComment: "//", TabSize: 2, InsertSpaces: true},
	{ID: "astro", Name: "Astro", Extensions: []string{"astro"}, UIMode: "Astro", Core: "astro", BlockComment: [2]string{"<!--", "-->"}, TabSize: 2, InsertSpaces: true, LanguageServers: []string{"astro-ls", "oxlint", "eslint"}},
	{ID: "bat", Name: "Batch", Extensions: []string{"bat", "cmd"}, UIMode: "Batch", Core: "bat", LineComment: "@REM", TabSize: 4, InsertSpaces: true},
	{ID: "c", Name: "C", Extensions: []string{"c", "h"}, UIMode: "C", Core: "c", LineComment: "//", BlockComment: [2]string{"/*", "*/"}, TabSize: 4, InsertSpaces: true, TreeSitter: "c", LanguageServers: []string{"clangd"}, Debugger: true},
	{ID: "clojure", Name: "Clojure", Extensions: []string{"clj", "cljs", "cljc", "cljd", "edn", "bb"}, Filenames: []string{"deps.edn", "project.clj", "bb.edn"}, UIMode: "Clojure", Core: "clojure", LineComment: ";", TabSize: 2, InsertSpaces: true, LanguageServers: []string{"clojure-lsp"}},
	{ID: "cmake", Name: "CMake", Extensions: []string{"cmake"}, Filenames: []string{"CMakeLists.txt", "CMakeCache.txt"}, UIMode: "CMake", Core: "cmake", LineComment: "#", BlockComment: [2]string{"#[[", "]]"}, TabSize: 2, InsertSpaces: true, TreeSitter: "cmake", LanguageServers: []string{"neocmakelsp", "cmake-language-server"}},
	{ID: "cpp", Name: "C++", Extensions: []string{"cpp", "cc", "cxx", "c++", "hpp", "hh", "hxx", "h++", "ipp", "tpp", "inl", "ixx", "cppm"}, UIMode: "C++", Core: "cpp", LineComment: "//", BlockComment: [2]string{"/*", "*/"}, TabSize: 4, InsertSpaces: true, TreeSitter: "cpp", LanguageServers: []string{"clangd"}, Debugger: true},
	{ID: "csharp", Name: "C#", Extensions: []string{"cs", "csx"}, UIMode: "C#", Core: "csharp", LineComment: "//", BlockComment: [2]string{"/*", "*/"}, TabSize: 4, InsertSpaces: true, TreeSitter: "csharp", LanguageServers: []string{"csharp-ls"}, Debugger: true},
	{ID: "css", Name: "CSS", Extensions: []string{"css", "pcss", "postcss"}, UIMode: "CSS", Core: "css", BlockComment: [2]string{"/*", "*/"}, TabSize: 2, InsertSpaces: true, TreeSitter: "css", LanguageServers: []string{"vscode-css-language-server"}},
	{ID: "d", Name: "D", Extensions: []string{"d", "di"}, UIMode: "D", Core: "d", LineComment: "//", BlockComment: [2]string{"/*", "*/"}, TabSize: 4, InsertSpaces: true, LanguageServers: []string{"serve-d"}},
	{ID: "dart", Name: "Dart", Extensions: []string{"dart"}, UIMode: "Dart", Core: "dart", LineComment: "//", BlockComment: [2]string{"/*", "*/"}, TabSize: 2, InsertSpaces: true, TreeSitter: "dart", LanguageServers: []string{"dart"}, Debugger: true},
	{ID: "diff", Name: "Diff", Extensions: []string{"diff", "patch", "rej"}, UIMode: "diff", Core: "diff", TabSize: 4},
	{ID: "django-html", Name: "Django Template", Extensions: []string{"djhtml"}, UIMode: "Django Template", Core: "html", BlockComment: [2]string{"{#", "#}"}, TabSize: 2, InsertSpaces: true, TreeSitter: "html", LanguageServers: []string{"vscode-html-language-server"}},
	{ID: "dockerfile", Name: "Dockerfile", Extensions: []string{"dockerfile", "containerfile"}, Filenames: []string{"Dockerfile", "Containerfile", "Dockerfile.*", "Containerfile.*", "*.Dockerfile"}, UIMode: "Dockerfile", Core: "dockerfile", LineComment: "#", TabSize: 4, InsertSpaces: true, LanguageServers: []string{"dockerfile-language-server"}},
	{ID: "dotenv", Name: ".env", Extensions: []string{"env"}, Filenames: []string{".env", ".env.*", ".flaskenv"}, UIMode: "Properties files", Core: "properties", LineComment: "#", TabSize: 2, InsertSpaces: true},
	{ID: "elixir", Name: "Elixir", Extensions: []string{"ex", "exs"}, Filenames: []string{"mix.lock"}, Core: "elixir", LineComment: "#", TabSize: 2, InsertSpaces: true, TreeSitter: "elixir", LanguageServers: []string{"elixir-ls"}, Debugger: true},
	{ID: "erb", Name: "ERB", Extensions: []string{"erb", "html.erb", "rhtml", "js.erb", "text.erb", "xml.erb", "json.erb"}, UIMode: "ERB", Core: "erb", BlockComment: [2]string{"<%#", "%>"}, TabSize: 2, InsertSpaces: true},
	{ID: "erlang", Name: "Erlang", Extensions: []string{"erl", "hrl", "escript", "app.src"}, Filenames: []string{"rebar.config", "rebar.lock", "sys.config"}, UIMode: "Erlang", Core: "erlang", LineComment: "%", TabSize: 4, InsertSpaces: true, LanguageServers: []string{"elp"}},
	{ID: "fsharp", Name: "F#", Extensions: []string{"fs", "fsi", "fsx"}, UIMode: "F#", Core: "fsharp", LineComment: "//", BlockComment: [2]string{"(*", "*)"}, TabSize: 4, InsertSpaces: true, LanguageServers: []string{"fsautocomplete"}, Debugger: true},
	{ID: "gitattributes", Name: "Git Attributes", Filenames: []string{".gitattributes"}, Core: "properties", LineComment: "#", TabSize: 2, InsertSpaces: true},
	{ID: "gdscript", Name: "GDScript", Extensions: []string{"gd"}, Core: "gdscript", LineComment: "#", TabSize: 4, TreeSitter: "gdscript", LanguageServers: []string{"godot"}, Debugger: true},
	{ID: "gdshader", Name: "Godot Shader", Extensions: []string{"gdshader", "gdshaderinc"}, UIMode: "C", Core: "c", LineComment: "//", BlockComment: [2]string{"/*", "*/"}, TabSize: 4},
	{ID: "go", Name: "Go", Extensions: []string{"go"}, UIMode: "Go", Core: "go", LineComment: "//", BlockComment: [2]string{"/*", "*/"}, TabSize: 4, TreeSitter: "go", LanguageServers: []string{"gopls"}, Debugger: true},
	{ID: "go.mod", Name: "Go Module", Filenames: []string{"go.mod"}, UIMode: "GoMod", Core: "gomod", LineComment: "//", TabSize: 4, LanguageServers: []string{"gopls"}},
	{ID: "go.work", Name: "Go Workspace", Filenames: []string{"go.work"}, UIMode: "GoMod", Core: "gomod", LineComment: "//", TabSize: 4, LanguageServers: []string{"gopls"}},
	{ID: "godot-resource", Name: "Godot Resource", Extensions: []string{"tscn", "tres", "godot", "import"}, Filenames: []string{"export_presets.cfg", "override.cfg", "extension_list.cfg"}, UIMode: "Properties files", Core: "properties", LineComment: ";", TabSize: 4},
	{ID: "graphql", Name: "GraphQL", Extensions: []string{"graphql", "graphqls", "gql"}, Core: "graphql", LineComment: "#", TabSize: 2, InsertSpaces: true, TreeSitter: "graphql", LanguageServers: []string{"graphql-lsp"}},
	{ID: "groovy", Name: "Groovy", Extensions: []string{"groovy", "gradle", "gvy", "gy", "gsh"}, Filenames: []string{"Jenkinsfile"}, UIMode: "Groovy", Core: "groovy", LineComment: "//", BlockComment: [2]string{"/*", "*/"}, TabSize: 4, InsertSpaces: true, LanguageServers: []string{"groovy-language-server"}},
	{ID: "haskell", Name: "Haskell", Extensions: []string{"hs", "hs-boot", "hsig"}, UIMode: "Haskell", Core: "haskell", LineComment: "--", BlockComment: [2]string{"{-", "-}"}, TabSize: 2, InsertSpaces: true, TreeSitter: "haskell", LanguageServers: []string{"haskell-language-server"}},
	{ID: "hcl", Name: "HCL", Extensions: []string{"hcl", "nomad", "pkr.hcl"}, Core: "hcl", LineComment: "#", BlockComment: [2]string{"/*", "*/"}, TabSize: 2, InsertSpaces: true, TreeSitter: "hcl"},
	{ID: "html", Name: "HTML", Extensions: []string{"html", "htm", "xhtml", "shtml", "html.tmpl"}, UIMode: "HTML", Core: "html", BlockComment: [2]string{"<!--", "-->"}, TabSize: 2, InsertSpaces: true, TreeSitter: "html", LanguageServers: []string{"angular-language-server", "vscode-html-language-server"}},
	{ID: "http", Name: "HTTP Request", Extensions: []string{"http", "rest"}, UIMode: "HTTP", Core: "http", LineComment: "#", TabSize: 2, InsertSpaces: true},
	{ID: "ignore", Name: "Ignore File", Extensions: []string{"gitignore", "dockerignore"}, Filenames: []string{".gitignore", ".gitignore_global", ".dockerignore", ".npmignore", ".prettierignore", ".eslintignore", ".stylelintignore", ".hgignore", ".ignore", ".rgignore", ".fdignore", ".helmignore", ".vscodeignore", ".gcloudignore", ".slugignore", ".cfignore", ".bzrignore", ".containerignore"}, Core: "ignore", LineComment: "#", TabSize: 2, InsertSpaces: true},
	{ID: "ini", Name: "INI", Extensions: []string{"ini", "cfg", "conf", "cnf", "inf", "reg", "url", "desktop", "service", "socket", "timer"}, Filenames: []string{".editorconfig", ".gitconfig", ".gitmodules", ".npmrc", ".yarnrc", ".pypirc", ".flake8", ".pylintrc", "pylintrc", "setup.cfg", "tox.ini", "pytest.ini", "mypy.ini", ".coveragerc", "php.ini", ".hgrc", "hgrc", "credentials"}, UIMode: "Properties files", Core: "ini", LineComment: ";", TabSize: 4, InsertSpaces: true},
	{ID: "java", Name: "Java", Extensions: []string{"java"}, UIMode: "Java", Core: "java", LineComment: "//", BlockComment: [2]string{"/*", "*/"}, TabSize: 4, InsertSpaces: true, TreeSitter: "java", LanguageServers: []string{"jdtls", "kmp-lsp"}, Debugger: true},
	{ID: "javascript", Name: "JavaScript", Extensions: []string{"js", "mjs", "cjs"}, UIMode: "JavaScript", Core: "javascript", LineComment: "//", BlockComment: [2]string{"/*", "*/"}, TabSize: 2, InsertSpaces: true, TreeSitter: "javascript", LanguageServers: []string{"vtsls", "typescript-language-server", "oxlint", "eslint"}, Debugger: true},
	{ID: "javascriptreact", Name: "JavaScript JSX", Extensions: []string{"jsx"}, UIMode: "JSX", Core: "javascriptreact", LineComment: "//", BlockComment: [2]string{"/*", "*/"}, TabSize: 2, InsertSpaces: true, TreeSitter: "javascript", LanguageServers: []string{"vtsls", "typescript-language-server", "oxlint", "eslint"}, Debugger: true},
	{ID: "json", Name: "JSON", Extensions: []string{"json", "geojson", "topojson", "webmanifest", "har", "avsc", "jsonl", "ndjson", "json.map"}, Filenames: []string{".prettierrc", ".watchmanconfig", "composer.lock", ".arcconfig"}, UIMode: "JSON", Core: "json", TabSize: 2, InsertSpaces: true, TreeSitter: "json", LanguageServers: []string{"vscode-json-language-server"}},
	{ID: "jsonc", Name: "JSON with Comments", Extensions: []string{"jsonc", "code-workspace", "code-snippets"}, Filenames: []string{"tsconfig.json", "jsconfig.json", "tsconfig.*.json", "jsconfig.*.json", ".eslintrc", ".eslintrc.json", ".jshintrc", ".babelrc", ".swcrc", "devcontainer.json", ".devcontainer.json", "launch.json", "tasks.json", "settings.json", "extensions.json", "keybindings.json", "tslint.json", "api-extractor.json", "typedoc.json", "deno.json", "biome.json", "turbo.json", "language-configuration.json"}, UIMode: "JSON", Core: "jsonc", LineComment: "//", BlockComment: [2]string{"/*", "*/"}, TabSize: 2, InsertSpaces: true, TreeSitter: "json", LanguageServers: []string{"vscode-json-language-server"}},
	{ID: "jsp", Name: "JSP", Extensions: []string{"jsp", "jspx", "jspf", "jsph", "tag", "tagf", "tagx"}, UIMode: "JSP", Core: "jsp", BlockComment: [2]string{"<%--", "--%>"}, TabSize: 2, InsertSpaces: true},
	{ID: "julia", Name: "Julia", Extensions: []string{"jl"}, UIMode: "Julia", Core: "julia", LineComment: "#", BlockComment: [2]string{"#=", "=#"}, TabSize: 4, InsertSpaces: true, LanguageServers: []string{"julia-languageserver"}},
	{ID: "kotlin", Name: "Kotlin", Extensions: []string{"kt", "kts"}, UIMode: "Kotlin", Core: "kotlin", LineComment: "//", BlockComment: [2]string{"/*", "*/"}, TabSize: 4, InsertSpaces: true, TreeSitter: "kotlin", LanguageServers: []string{"kotlin-lsp", "kotlin-language-server", "kmp-lsp"}, Debugger: true},
	{ID: "latex", Name: "LaTeX", Extensions: []string{"tex", "ltx", "sty", "cls", "dtx", "ins", "bbx", "cbx"}, UIMode: "LaTeX", Core: "latex", LineComment: "%", TabSize: 2, InsertSpaces: true, LanguageServers: []string{"texlab"}},
	{ID: "less", Name: "Less", Extensions: []string{"less"}, UIMode: "LESS", Core: "css", LineComment: "//", BlockComment: [2]string{"/*", "*/"}, TabSize: 2, InsertSpaces: true, LanguageServers: []string{"vscode-css-language-server"}},
	{ID: "log", Name: "Log", Extensions: []string{"log"}, UIMode: "Log", Core: "log", TabSize: 4, InsertSpaces: true},
	{ID: "lua", Name: "Lua", Extensions: []string{"lua", "luau", "rockspec"}, Filenames: []string{".luacheckrc"}, UIMode: "Lua", Core: "lua", LineComment: "--", BlockComment: [2]string{"--[[", "]]"}, TabSize: 2, InsertSpaces: true, TreeSitter: "lua", LanguageServers: []string{"lua-language-server"}},
	{ID: "makefile", Name: "Makefile", Extensions: []string{"mk", "mak", "make"}, Filenames: []string{"Makefile", "makefile", "GNUmakefile", "BSDmakefile", "Makefile.am", "Makefile.in", "Kbuild"}, Core: "makefile", LineComment: "#", TabSize: 4, TreeSitter: "make"},
	{ID: "markdown", Name: "Markdown", Extensions: []string{"md", "markdown", "mdown", "mkd", "mkdn", "mdwn", "mdx", "ronn", "workbook"}, UIMode: "Markdown", Core: "markdown", BlockComment: [2]string{"<!--", "-->"}, TabSize: 2, InsertSpaces: true, TreeSitter: "markdown", LanguageServers: []string{"marksman"}},
	{ID: "nix", Name: "Nix", Extensions: []string{"nix"}, Core: "nix", LineComment: "#", BlockComment: [2]string{"/*", "*/"}, TabSize: 2, InsertSpaces: true, TreeSitter: "nix", LanguageServers: []string{"nil", "nixd"}},
	{ID: "objective-c", Name: "Objective-C", Extensions: []string{"m"}, UIMode: "Objective-C", Core: "cpp", LineComment: "//", BlockComment: [2]string{"/*", "*/"}, TabSize: 4, InsertSpaces: true, LanguageServers: []string{"clangd"}, Debugger: true},
	{ID: "objective-cpp", Name: "Objective-C++", Extensions: []string{"mm"}, UIMode: "Objective-C++", Core: "cpp", LineComment: "//", BlockComment: [2]string{"/*", "*/"}, TabSize: 4, InsertSpaces: true, LanguageServers: []string{"clangd"}, Debugger: true},
	{ID: "ocaml", Name: "OCaml", Extensions: []string{"ml", "mll", "mly"}, UIMode: "OCaml", Core: "ocaml", BlockComment: [2]string{"(*", "*)"}, TabSize: 2, InsertSpaces: true, TreeSitter: "ocaml", LanguageServers: []string{"ocamllsp"}, Debugger: true},
	{ID: "ocaml.interface", Name: "OCaml Interface", Extensions: []string{"mli"}, UIMode: "OCaml", Core: "ocaml", BlockComment: [2]string{"(*", "*)"}, TabSize: 2, InsertSpaces: true, TreeSitter: "ocaml_interface", LanguageServers: []string{"ocamllsp"}},
	{ID: "perl", Name: "Perl", Extensions: []string{"pl", "pm", "pod", "psgi", "cgi"}, Filenames: []string{"cpanfile", "Makefile.PL", "Build.PL"}, UIMode: "Perl", Core: "perl", LineComment: "#", TabSize: 4, InsertSpaces: true, LanguageServers: []string{"perlnavigator"}},
	{ID: "php", Name: "PHP", Extensions: []string{"php", "phtml", "php3", "php4", "php5", "phps", "phpt"}, UIMode: "PHP", Core: "php", LineComment: "//", BlockComment: [2]string{"/*", "*/"}, TabSize: 4, InsertSpaces: true, TreeSitter: "php", LanguageServers: []string{"intelephense", "phpactor", "laravel-lsp"}, Debugger: true},
	{ID: "plaintext", Name: "Plain Text", Extensions: []string{"txt", "text"}, Filenames: []string{"LICENSE", "LICENCE", "COPYING", "AUTHORS", "NOTICE", "CODEOWNERS", "README", "CHANGELOG", "CONTRIBUTORS", "requirements.txt"}, Core: "plaintext", TabSize: 4, InsertSpaces: true},
	{ID: "powershell", Name: "PowerShell", Extensions: []string{"ps1", "psm1", "psd1", "pssc", "psrc"}, UIMode: "PowerShell", Core: "powershell", LineComment: "#", BlockComment: [2]string{"<#", "#>"}, TabSize: 4, InsertSpaces: true, LanguageServers: []string{"powershell-editor-services"}},
	{ID: "prisma", Name: "Prisma", Extensions: []string{"prisma"}, Core: "prisma", LineComment: "//", TabSize: 2, InsertSpaces: true, TreeSitter: "prisma", LanguageServers: []string{"prisma-language-server"}},
	{ID: "properties", Name: "Properties", Extensions: []string{"properties", "prefs"}, Filenames: []string{"gradle.properties", "local.properties"}, UIMode: "Properties files", Core: "properties", LineComment: "#", TabSize: 4, InsertSpaces: true},
	{ID: "proto", Name: "Protocol Buffers", Extensions: []string{"proto"}, UIMode: "ProtoBuf", Core: "proto", LineComment: "//", BlockComment: [2]string{"/*", "*/"}, TabSize: 2, InsertSpaces: true, TreeSitter: "proto", LanguageServers: []string{"buf"}},
	{ID: "python", Name: "Python", Extensions: []string{"py", "pyi", "pyw"}, Filenames: []string{"SConstruct", "SConscript"}, UIMode: "Python", Core: "python", LineComment: "#", TabSize: 4, InsertSpaces: true, TreeSitter: "python", LanguageServers: []string{"basedpyright", "pyright", "ty", "pyrefly", "ruff"}, Debugger: true},
	{ID: "r", Name: "R", Extensions: []string{"r", "rprofile"}, Filenames: []string{".Rprofile"}, UIMode: "R", Core: "r", LineComment: "#", TabSize: 2, InsertSpaces: true, LanguageServers: []string{"r-languageserver"}},
	{ID: "restructuredtext", Name: "reStructuredText", Extensions: []string{"rst"}, UIMode: "reStructuredText", Core: "restructuredtext", LineComment: "..", TabSize: 3, InsertSpaces: true},
	{ID: "ruby", Name: "Ruby", Extensions: []string{"rb", "rake", "gemspec", "ru", "rbi", "jbuilder", "podspec"}, Filenames: []string{"Gemfile", "Rakefile", "Podfile", "Vagrantfile", "Brewfile", "Guardfile", "Fastfile", "Appfile", "Capfile", "Dangerfile", "Steepfile", ".irbrc", ".pryrc"}, UIMode: "Ruby", Core: "ruby", LineComment: "#", BlockComment: [2]string{"=begin", "=end"}, TabSize: 2, InsertSpaces: true, TreeSitter: "ruby", LanguageServers: []string{"ruby-lsp", "solargraph"}, Debugger: true},
	{ID: "rust", Name: "Rust", Extensions: []string{"rs"}, UIMode: "Rust", Core: "rust", LineComment: "//", BlockComment: [2]string{"/*", "*/"}, TabSize: 4, InsertSpaces: true, TreeSitter: "rust", LanguageServers: []string{"rust-analyzer"}, Debugger: true},
	{ID: "sass", Name: "Sass", Extensions: []string{"sass"}, UIMode: "Sass", Core: "sass", LineComment: "//", BlockComment: [2]string{"/*", "*/"}, TabSize: 2, InsertSpaces: true},
	{ID: "scala", Name: "Scala", Extensions: []string{"scala", "sc", "sbt", "mill"}, UIMode: "Scala", Core: "scala", LineComment: "//", BlockComment: [2]string{"/*", "*/"}, TabSize: 2, InsertSpaces: true, TreeSitter: "scala", LanguageServers: []string{"metals"}},
	{ID: "scss", Name: "SCSS", Extensions: []string{"scss"}, UIMode: "SCSS", Core: "css", LineComment: "//", BlockComment: [2]string{"/*", "*/"}, TabSize: 2, InsertSpaces: true, LanguageServers: []string{"vscode-css-language-server"}},
	{ID: "shellscript", Name: "Shell Script", Extensions: []string{"sh", "bash", "zsh", "ksh", "ash", "dash", "bats", "command"}, Filenames: []string{".bashrc", ".bash_profile", ".bash_login", ".bash_logout", ".bash_aliases", ".profile", ".zshrc", ".zshenv", ".zprofile", ".zlogin", ".zlogout", ".kshrc", "PKGBUILD", "APKBUILD", ".envrc"}, UIMode: "Shell", Core: "shellscript", LineComment: "#", TabSize: 2, InsertSpaces: true, TreeSitter: "bash", LanguageServers: []string{"bash-language-server"}, Debugger: true},
	{ID: "sql", Name: "SQL", Extensions: []string{"sql", "ddl", "dml", "psql", "pgsql", "mysql"}, UIMode: "SQL", Core: "sql", LineComment: "--", BlockComment: [2]string{"/*", "*/"}, TabSize: 4, InsertSpaces: true, LanguageServers: []string{"sqls"}},
	{ID: "svelte", Name: "Svelte", Extensions: []string{"svelte"}, UIMode: "HTML", Core: "svelte", BlockComment: [2]string{"<!--", "-->"}, TabSize: 2, InsertSpaces: true, LanguageServers: []string{"svelte-language-server", "oxlint", "eslint"}},
	{ID: "swift", Name: "Swift", Extensions: []string{"swift", "swiftinterface"}, UIMode: "Swift", Core: "swift", LineComment: "//", BlockComment: [2]string{"/*", "*/"}, TabSize: 4, InsertSpaces: true, TreeSitter: "swift", LanguageServers: []string{"sourcekit-lsp"}, Debugger: true},
	{ID: "terraform", Name: "Terraform", Extensions: []string{"tf", "tofu", "tftest.hcl"}, Filenames: []string{".terraformrc", "terraform.rc"}, Core: "hcl", LineComment: "#", BlockComment: [2]string{"/*", "*/"}, TabSize: 2, InsertSpaces: true, TreeSitter: "hcl", LanguageServers: []string{"terraform-ls"}},
	{ID: "terraform-vars", Name: "Terraform Variables", Extensions: []string{"tfvars"}, Core: "hcl", LineComment: "#", BlockComment: [2]string{"/*", "*/"}, TabSize: 2, InsertSpaces: true, TreeSitter: "hcl", LanguageServers: []string{"terraform-ls"}},
	{ID: "toml", Name: "TOML", Extensions: []string{"toml"}, Filenames: []string{"Cargo.lock", "Pipfile", "poetry.lock", "uv.lock", "pdm.lock", "Gopkg.lock"}, UIMode: "TOML", Core: "toml", LineComment: "#", TabSize: 2, InsertSpaces: true, TreeSitter: "toml", LanguageServers: []string{"taplo"}},
	{ID: "typescript", Name: "TypeScript", Extensions: []string{"ts", "mts", "cts"}, UIMode: "TypeScript", Core: "typescript", LineComment: "//", BlockComment: [2]string{"/*", "*/"}, TabSize: 2, InsertSpaces: true, TreeSitter: "typescript", LanguageServers: []string{"vtsls", "typescript-language-server", "angular-language-server", "oxlint", "eslint"}, Debugger: true},
	{ID: "typescriptreact", Name: "TypeScript JSX", Extensions: []string{"tsx"}, UIMode: "TSX", Core: "typescriptreact", LineComment: "//", BlockComment: [2]string{"/*", "*/"}, TabSize: 2, InsertSpaces: true, TreeSitter: "tsx", LanguageServers: []string{"vtsls", "typescript-language-server", "oxlint", "eslint"}, Debugger: true},
	{ID: "vue", Name: "Vue", Extensions: []string{"vue"}, UIMode: "Vue", Core: "vue", BlockComment: [2]string{"<!--", "-->"}, TabSize: 2, InsertSpaces: true, LanguageServers: []string{"vue-language-server", "vtsls", "oxlint", "eslint"}},
	{ID: "xml", Name: "XML", Extensions: []string{"xml", "xsd", "xsl", "xslt", "svg", "plist", "csproj", "fsproj", "vbproj", "props", "targets", "nuspec", "xaml", "axaml", "resx", "wsdl", "fxml", "iml", "jnlp", "rss", "atom", "xliff", "xlf", "storyboard", "xib", "tld", "pom"}, Filenames: []string{"NuGet.Config", "Directory.Build.targets", "web.config", "app.config"}, UIMode: "XML", Core: "xml", BlockComment: [2]string{"<!--", "-->"}, TabSize: 2, InsertSpaces: true, TreeSitter: "xml", LanguageServers: []string{"lemminx"}},
	{ID: "yaml", Name: "YAML", Extensions: []string{"yaml", "yml"}, Filenames: []string{".clang-format", ".clang-tidy", ".clangd", ".gemrc", "pnpm-lock.yaml", "CITATION.cff"}, UIMode: "YAML", Core: "yaml", LineComment: "#", TabSize: 2, InsertSpaces: true, TreeSitter: "yaml", LanguageServers: []string{"yaml-language-server"}},
	{ID: "zig", Name: "Zig", Extensions: []string{"zig", "zon"}, Core: "zig", LineComment: "//", TabSize: 4, InsertSpaces: true, TreeSitter: "zig", LanguageServers: []string{"zls"}, Debugger: true},
}

var (
	languageOnce   sync.Once
	languageByID   map[string]*languageDefinition
	languageByName map[string]*languageDefinition
	languageByExt  map[string]*languageDefinition
	languageFiles  map[string]*languageDefinition
	languageGlobs  []*languageDefinition
)

func initLanguages() {
	languageByID = make(map[string]*languageDefinition, len(languageCatalog))
	languageByName = make(map[string]*languageDefinition, len(languageCatalog))
	languageByExt = make(map[string]*languageDefinition)
	languageFiles = make(map[string]*languageDefinition)
	for i := range languageCatalog {
		definition := &languageDefinition{Language: languageCatalog[i]}
		languageByID[strings.ToLower(definition.ID)] = definition
		languageByName[normalizedLanguage(definition.Name)] = definition
		languageByName[normalizedLanguage(definition.ID)] = definition
		if definition.UIMode != "" {
			languageByName[normalizedLanguage(definition.UIMode)] = definition
		}
		for _, extension := range definition.Extensions {
			extension = strings.TrimPrefix(strings.ToLower(extension), ".")
			if extension != "" {
				languageByExt[extension] = definition
			}
		}
		for _, name := range definition.Filenames {
			if strings.ContainsAny(name, "*?") {
				languageGlobs = append(languageGlobs, definition)
				languageByName[strings.ToLower(name)] = definition
				continue
			}
			languageFiles[strings.ToLower(name)] = definition
		}
		if definition.UIMode != "" {
		}
	}
	sort.SliceStable(languageGlobs, func(i, j int) bool {
		return len(languageGlobs[i].Filenames) > len(languageGlobs[j].Filenames)
	})
}

// Languages returns OpenIDEA's language contributions. The returned slice is
// independent, so callers can filter or sort it safely.
func Languages() []Language {
	out := make([]Language, len(languageCatalog))
	for i := range languageCatalog {
		out[i] = cloneLanguage(languageCatalog[i])
	}
	return out
}

// LanguageByID returns the contribution for OpenIDEA's language id.
func LanguageByID(id string) *Language {
	languageOnce.Do(initLanguages)
	if definition := languageByID[strings.ToLower(strings.TrimSpace(id))]; definition != nil {
		language := cloneLanguage(definition.Language)
		return &language
	}
	return nil
}

// LanguageNamed resolves a language name, id, UI mode or common extension.
func LanguageNamed(name string) *Language {
	languageOnce.Do(initLanguages)
	key := normalizedLanguage(name)
	definition := languageByName[key]
	if definition == nil {
		definition = languageByExt[strings.TrimPrefix(strings.ToLower(strings.TrimSpace(name)), ".")]
	}
	if definition != nil {
		language := cloneLanguage(definition.Language)
		return &language
	}
	return nil
}

// LanguageFor detects a contribution exactly as OpenIDEA's registry does:
// file name, file-name pattern, then the longest compound extension.
func LanguageFor(filename string) *Language {
	languageOnce.Do(initLanguages)
	name := path.Base(strings.ReplaceAll(filename, "\\", "/"))
	if definition := languageFiles[strings.ToLower(name)]; definition != nil {
		language := cloneLanguage(definition.Language)
		return &language
	}
	for _, definition := range languageGlobs {
		for _, pattern := range definition.Filenames {
			if strings.ContainsAny(pattern, "*?") && wildcardMatch(pattern, name) {
				language := cloneLanguage(definition.Language)
				return &language
			}
		}
	}
	lower := strings.ToLower(name)
	for rest := lower; ; {
		dot := strings.IndexByte(rest, '.')
		if dot < 0 {
			break
		}
		rest = rest[dot+1:]
		if definition := languageByExt[rest]; definition != nil {
			language := cloneLanguage(definition.Language)
			return &language
		}
	}
	return nil
}

// LexerNameFor returns a Chroma lexer name for a contribution, "plaintext"
// for a deliberately plain language, or "" when an exact renderer is absent.
func LexerNameFor(language *Language) string {
	if language == nil {
		return ""
	}
	names := map[string]string{
		"c": "C", "cpp": "C++", "csharp": "C#", "css": "CSS", "d": "D",
		"dart": "Dart", "diff": "Diff", "dockerfile": "Dockerfile", "elixir": "Elixir",
		"erb": "HTML+ERB", "erlang": "Erlang", "fsharp": "FSharp", "go": "Go",
		"graphql": "GraphQL", "groovy": "Groovy", "haskell": "Haskell", "hcl": "HCL",
		"html": "HTML", "http": "HTTP", "ini": "INI", "java": "Java", "javascript": "JavaScript",
		"javascriptreact": "React", "json": "JSON", "jsonc": "JSON", "julia": "Julia",
		"kotlin": "Kotlin", "latex": "TeX", "lua": "Lua", "makefile": "Makefile",
		"markdown": "markdown", "nix": "Nix", "objective-c": "Objective-C",
		"ocaml": "OCaml", "perl": "Perl", "php": "PHP", "powershell": "PowerShell",
		"properties": "properties", "proto": "Protocol Buffer", "python": "Python",
		"r": "R", "restructuredtext": "reStructuredText", "ruby": "Ruby", "rust": "Rust",
		"sass": "Sass", "scala": "Scala", "scss": "SCSS", "shellscript": "Bash",
		"sql": "SQL", "svelte": "Svelte", "swift": "Swift", "terraform": "HCL",
		"toml": "TOML", "typescript": "TypeScript", "typescriptreact": "TypeScript",
		"vue": "Vue", "xml": "XML", "yaml": "YAML", "zig": "Zig",
		"asciidoc": "AsciiDoc", "astro": "Astro", "bat": "Batchfile", "clojure": "Clojure",
		"cmake": "CMake", "django-html": "Django/Jinja", "dotenv": "properties",
		"gdscript": "GDScript", "godot-resource": "properties", "ignore": "properties",
		"jsp": "JSP", "prisma": "Prisma", "terraform-vars": "HCL",
		"plaintext": "plaintext",
	}
	if name := names[language.Core]; name != "" {
		return name
	}
	return ""
}

// MyGoLanguage returns a native editor language with OpenIDEA's detection and
// editing metadata. The tokenizer comes from Chroma when one exists; otherwise
// the language stays honest with metadata-only support.
func MyGoLanguage(language *Language) *editor.Language {
	if language == nil {
		return nil
	}
	base := editor.LanguageNamed(language.UIMode)
	if base == nil {
		base = editor.LanguageNamed(language.Name)
	}
	out := &editor.Language{}
	if base != nil {
		*out = *base
	} else {
		out.Brackets = []editor.BracketPair{{Open: "(", Close: ")"}, {Open: "[", Close: "]"}, {Open: "{", Close: "}"}}
		out.AutoClose = append(append([]editor.BracketPair(nil), out.Brackets...),
			editor.BracketPair{Open: "\"", Close: "\"", NotIn: []editor.TokenKind{editor.TokenString, editor.TokenComment}},
			editor.BracketPair{Open: "'", Close: "'", NotIn: []editor.TokenKind{editor.TokenString, editor.TokenComment}})
	}
	out.Name = language.Name
	out.Extensions = extensionPatterns(language.Extensions)
	out.Filenames = cloneStrings(language.Filenames)
	out.LineComment = language.LineComment
	out.BlockComment = language.BlockComment
	out.TabSize = max(1, language.TabSize)
	out.HardTabs = language.TabSize > 0 && !language.InsertSpaces
	out.Tokenizer = tokenizerFor(language)
	if out.Tokenizer == nil {
		if language.Core != "plaintext" {
			out.Highlighter = highlighterFor(language)
		}
	}
	return out
}

func tokenizerFor(language *Language) editor.Tokenizer {
	if language == nil || language.Core == "plaintext" {
		return nil
	}
	if base := editor.LanguageNamed(language.UIMode); base != nil && base.Tokenizer != nil {
		return base.Tokenizer
	}
	if base := editor.LanguageNamed(language.Name); base != nil && base.Tokenizer != nil {
		return base.Tokenizer
	}
	return nil
}

func highlighterFor(language *Language) func(editor.Snapshot, func()) editor.Highlighter {
	name := LexerNameFor(language)
	if name == "" {
		return nil
	}
	lexer := lexers.Get(name)
	if lexer == nil {
		return nil
	}
	return func(text editor.Snapshot, invalidate func()) editor.Highlighter {
		return &chromaHighlighter{highlighter: New(chroma.Coalesce(lexer), invalidate), text: text}
	}
}

type chromaHighlighter struct {
	highlighter *Highlighter
	text        editor.Snapshot
}

func (h *chromaHighlighter) Edit(_, next editor.Snapshot, _ []editor.Change) {
	h.text = next
}

func (h *chromaHighlighter) Highlight(last int, _ time.Duration) bool {
	h.highlighter.Request(snapshotSource{h.text}, 0, last)
	return h.highlighter.Gen() != 0
}

func (h *chromaHighlighter) Tokens(line int, text string, dst []editor.Token) ([]editor.Token, bool) {
	tokens := h.highlighter.Line(line, text)
	if tokens == nil {
		return dst, false
	}
	for _, token := range tokens {
		kind := highlightTokenKind(token.Kind)
		if kind == editor.TokenText {
			continue
		}
		dst = append(dst, editor.Token{End: token.End, Kind: kind})
	}
	return dst, true
}

func highlightTokenKind(kind Kind) editor.TokenKind {
	switch kind {
	case Keyword:
		return editor.TokenKeyword
	case Type:
		return editor.TokenType
	case Function, Builtin:
		return editor.TokenFunction
	case String:
		return editor.TokenString
	case Number:
		return editor.TokenNumber
	case Comment:
		return editor.TokenComment
	case Operator:
		return editor.TokenOperator
	case Punctuation:
		return editor.TokenPunctuation
	case Constant:
		return editor.TokenConstant
	case Tag:
		return editor.TokenTag
	case Attribute:
		return editor.TokenAttribute
	case Variable:
		return editor.TokenVariable
	case Heading:
		return editor.TokenHeading
	case Deleted:
		return editor.TokenDeleted
	case Inserted:
		return editor.TokenInserted
	case Strong:
		return editor.TokenStrong
	case Emph:
		return editor.TokenEmphasis
	case Mark:
		return editor.TokenMeta
	default:
		return editor.TokenText
	}
}

type snapshotSource struct{ snapshot editor.Snapshot }

func (s snapshotSource) Version() uint64             { return s.snapshot.Version() }
func (s snapshotSource) LineCount() int              { return s.snapshot.LineCount() }
func (s snapshotSource) Slice(start, end int) string { return s.snapshot.Slice(start, end) }
func (s snapshotSource) LineRange(line int) (int, int) {
	return s.snapshot.LineStart(line), s.snapshot.LineEnd(line)
}

func chromaKind(kind chroma.TokenType) editor.TokenKind {
	switch {
	case kind == chroma.KeywordType:
		return editor.TokenType
	case kind == chroma.KeywordConstant:
		return editor.TokenConstant
	case kind.InCategory(chroma.Keyword):
		return editor.TokenKeyword
	case kind == chroma.NameFunction || kind == chroma.NameFunctionMagic:
		return editor.TokenFunction
	case kind == chroma.NameClass || kind == chroma.NameNamespace:
		return editor.TokenType
	case kind == chroma.NameBuiltin || kind == chroma.NameBuiltinPseudo:
		return editor.TokenFunction
	case kind == chroma.NameTag:
		return editor.TokenTag
	case kind == chroma.NameAttribute || kind == chroma.NameDecorator:
		return editor.TokenAttribute
	case kind == chroma.NameConstant || kind == chroma.NameEntity:
		return editor.TokenConstant
	case kind == chroma.NameVariable || kind == chroma.NameVariableInstance || kind == chroma.NameVariableGlobal || kind == chroma.NameVariableClass:
		return editor.TokenVariable
	case kind == chroma.NameOther || kind == chroma.NameProperty:
		return editor.TokenProperty
	case kind.InSubCategory(chroma.LiteralString):
		return editor.TokenString
	case kind.InSubCategory(chroma.LiteralNumber):
		return editor.TokenNumber
	case kind == chroma.Literal || kind == chroma.LiteralDate:
		return editor.TokenConstant
	case kind.InCategory(chroma.Comment):
		return editor.TokenComment
	case kind.InCategory(chroma.Operator):
		return editor.TokenOperator
	case kind == chroma.Punctuation:
		return editor.TokenPunctuation
	case kind == chroma.GenericHeading || kind == chroma.GenericSubheading:
		return editor.TokenHeading
	}
	return editor.TokenText
}

func extensionPatterns(extensions []string) []string {
	out := make([]string, 0, len(extensions))
	for _, extension := range extensions {
		extension = strings.TrimPrefix(strings.TrimSpace(extension), ".")
		if extension != "" {
			out = append(out, "."+extension)
		}
	}
	return out
}

func cloneLanguage(language Language) Language {
	language.Extensions = cloneStrings(language.Extensions)
	language.Filenames = cloneStrings(language.Filenames)
	language.LanguageServers = cloneStrings(language.LanguageServers)
	return language
}

func cloneStrings(values []string) []string {
	return append([]string(nil), values...)
}

func normalizedLanguage(value string) string {
	return strings.ToLower(strings.NewReplacer("+", "plus", "#", "sharp", ".", "", "_", "", "-", "", " ", "").Replace(strings.TrimSpace(value)))
}

func wildcardMatch(pattern, name string) bool {
	pattern = strings.ToLower(pattern)
	name = strings.ToLower(name)
	if pattern == "" {
		return name == ""
	}
	if pattern[0] == '*' {
		for i := 0; i <= len(name); i++ {
			if wildcardMatch(pattern[1:], name[i:]) {
				return true
			}
		}
		return false
	}
	if pattern[0] == '?' {
		return name != "" && wildcardMatch(pattern[1:], name[1:])
	}
	return name != "" && pattern[0] == name[0] && wildcardMatch(pattern[1:], name[1:])
}
