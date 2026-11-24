package app

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

type nativeSettingsKind int

const utf8BOMSettingID = "editor.encodings.utf8Bom"

const (
	nativeSettingsToggle nativeSettingsKind = iota
	nativeSettingsText
	nativeSettingsNumber
	nativeSettingsSelect
	nativeSettingsList
	nativeSettingsMap
	nativeSettingsDisabledLanguages
	nativeSettingsPath
	nativeSettingsSDKPath
	nativeSettingsPythonInterpreters
	nativeSettingsJSON
	nativeSettingsCMakeProfiles
)

type nativeSettingsChoice struct {
	value string
	label string
}

type nativeSettingsField struct {
	key          string
	label        string
	hint         string
	placeholder  string
	kind         nativeSettingsKind
	defaultValue any
	choices      []nativeSettingsChoice
	min          float64
	max          float64
	step         float64
	// scope overrides the page's default for a mixed global/project page.
	scope string
	// sdkKind selects the sdk_detect list shown by an SDK path field.
	sdkKind   string
	directory bool
	// invert shows and edits the negation of the stored setting.
	invert bool
}

type nativeSettingsSection struct {
	title  string
	fields []nativeSettingsField
}

type nativeSettingsPageSpec struct {
	page         settingsPageSpec
	sections     []nativeSettingsSection
	defaultScope string
	projectOnly  bool
	note         string
}

func nativeChoice(value, label string) nativeSettingsChoice {
	return nativeSettingsChoice{value: value, label: label}
}

func nativeToggle(key, label string, value bool) nativeSettingsField {
	return nativeSettingsField{key: key, label: label, kind: nativeSettingsToggle, defaultValue: value}
}

func nativeInverseToggle(key, label string, value bool) nativeSettingsField {
	return nativeSettingsField{
		key: key, label: label, kind: nativeSettingsToggle, defaultValue: value, invert: true,
	}
}

func nativeText(key, label, placeholder, value string) nativeSettingsField {
	return nativeSettingsField{
		key: key, label: label, placeholder: placeholder,
		kind: nativeSettingsText, defaultValue: value,
	}
}

func nativeNumber(key, label string, value, min, max, step float64) nativeSettingsField {
	return nativeSettingsField{
		key: key, label: label, kind: nativeSettingsNumber, defaultValue: value,
		min: min, max: max, step: step,
	}
}

func nativeSelect(key, label, value string, choices ...nativeSettingsChoice) nativeSettingsField {
	return nativeSettingsField{
		key: key, label: label, kind: nativeSettingsSelect,
		defaultValue: value, choices: choices,
	}
}

func nativeList(key, label, placeholder string, value []string) nativeSettingsField {
	return nativeSettingsField{
		key: key, label: label, placeholder: placeholder,
		kind: nativeSettingsList, defaultValue: cloneStrings(value),
	}
}

func nativePath(key, label, placeholder string, directory bool) nativeSettingsField {
	return nativeSettingsField{
		key: key, label: label, placeholder: placeholder,
		kind: nativeSettingsPath, defaultValue: "", directory: directory,
	}
}

func nativeSDKPath(key, label, placeholder, kind string, directory bool) nativeSettingsField {
	return nativeSettingsField{
		key: key, label: label, placeholder: placeholder,
		kind: nativeSettingsSDKPath, defaultValue: "", sdkKind: kind, directory: directory,
	}
}

func nativeJSON(key, label, placeholder string, value any) nativeSettingsField {
	return nativeSettingsField{
		key: key, label: label, placeholder: placeholder,
		kind: nativeSettingsJSON, defaultValue: value,
	}
}

func nativeCMakeProfiles() nativeSettingsField {
	return nativeSettingsField{
		key: "cmake.profiles", label: "CMake profiles",
		kind: nativeSettingsCMakeProfiles, defaultValue: []any{},
	}
}

func nativeSettingsDefinitions() []nativeSettingsPageSpec {
	editorSections := func(id string, order int, sections ...nativeSettingsSection) nativeSettingsPageSpec {
		return nativeSettingsPageSpec{
			page: settingsPageSpec{
				id: id, group: "Editor", order: order, section: "nativeSettings",
			},
			defaultScope: "global",
			sections:     sections,
		}
	}
	toggle := nativeToggle
	text := nativeText
	number := nativeNumber
	selectField := nativeSelect
	list := nativeList

	pageDefs := []nativeSettingsPageSpec{
		{
			page: settingsPageSpec{
				id: settingsPageAgentMCPServer, title: "IDE MCP Server",
				group: "Agents", order: 470,
				keywords: []string{
					"mcp", "model context protocol", "agent", "run command",
					"agent tabs", "archive", "http", "stdio", "client configuration",
				},
				section: "nativeSettings",
			},
			defaultScope: "global",
			sections: []nativeSettingsSection{
				{fields: []nativeSettingsField{
					{
						key: "mcpServer.enabled", label: "Enable MCP server",
						kind: nativeSettingsToggle, defaultValue: false,
						hint: "Agents can use this project's tools while GoIDEA runs. The server listens on 127.0.0.1 and requires a token.",
					},
					{
						key:   "mcpServer.runWithoutConfirmation",
						label: "Run tools without confirmation (brave mode)",
						kind:  nativeSettingsToggle, defaultValue: false,
						hint: "Unattended agents can start run configurations and commands. Only application settings can turn this on.",
					},
					{
						key: "mcpServer.agentTabLimit", label: "Finished agent tabs to show",
						kind: nativeSettingsNumber, defaultValue: float64(defaultAgentRunTabLimit),
						min: 0, max: maxAgentRunTabLimit, step: 1,
						hint: "Older finished agent tabs move to the archive. Running tabs are never archived.",
					},
					{
						key: "mcpServer.agentArchiveLimit", label: "Archived agent tabs to keep",
						kind: nativeSettingsNumber, defaultValue: float64(defaultAgentRunArchiveLimit),
						min: 0, max: maxAgentRunArchiveLimit, step: 1,
						hint: "Archived agent tabs beyond this number are removed, oldest first.",
					},
				}},
				{title: "Endpoint"},
			},
		},
		editorSections(settingsPageAutoImport, 0, nativeSettingsSection{
			fields: []nativeSettingsField{
				selectField("editor.autoImport.importsOnPaste", "Insert imports on paste", "ask",
					nativeChoice("always", "Always"), nativeChoice("ask", "Ask"), nativeChoice("never", "Never")),
				toggle("editor.autoImport.addUnambiguous", "Add unambiguous imports on the fly", false),
				toggle("editor.autoImport.optimizeOnTheFly", "Optimize imports on the fly", false),
			},
		}),
		{
			page: settingsPageSpec{
				id: settingsPageBuildConsole, title: "Console",
				group: "Build, Execution, Deployment", order: 360,
				keywords: []string{"run output", "encoding", "fold", "buffer", "colour", "log level"},
				section:  "nativeSettings",
			},
			sections: []nativeSettingsSection{
				{
					fields: []nativeSettingsField{
						toggle("console.overrideBuffer", "Override console cycle buffer size", false),
						number("console.bufferKb", "Console cycle buffer size", 1024, 16, 1_048_576, 16),
						toggle("console.softWrap", "Use soft wraps in console", false),
						selectField("console.encoding", "Default encoding", "UTF-8",
							nativeChoices(
								"UTF-8", "UTF-16LE", "UTF-16BE", "ISO-8859-1", "windows-1250",
								"windows-1251", "windows-1252", "GBK", "GB18030", "Big5",
								"Shift_JIS", "EUC-JP", "EUC-KR", "KOI8-R",
							)...),
						toggle("console.logColors.enabled", "Colour log lines by level", false),
						selectField("console.logColors.error", "Error", "var(--ansi-red)", nativeConsoleColors()...),
						selectField("console.logColors.warn", "Warning", "var(--ansi-yellow)", nativeConsoleColors()...),
						selectField("console.logColors.info", "Info", "var(--ansi-green)", nativeConsoleColors()...),
						selectField("console.logColors.debug", "Debug", "var(--ansi-cyan)", nativeConsoleColors()...),
						selectField("console.logColors.trace", "Trace", "var(--ansi-magenta)", nativeConsoleColors()...),
					},
				},
				{
					title: "Folding",
					fields: []nativeSettingsField{
						list("console.foldPatterns", "Fold console lines that contain", "at org.example.", nil),
					},
				},
			},
		},
		{
			page: settingsPageSpec{
				id: settingsPageGradle, title: "Gradle",
				group: "Build, Execution, Deployment", order: 358,
				keywords: []string{
					"build and run using", "delegate", "JavaExec", "Gradle JVM", "JDK",
					"JAVA_HOME", "distribution", "wrapper", "gradlew", "local installation",
					"Gradle home", "Gradle user home", "GRADLE_USER_HOME", "mise", "asdf",
				},
				section: "nativeSettings",
			},
			defaultScope: "project",
			projectOnly:  true,
			sections: []nativeSettingsSection{{
				fields: []nativeSettingsField{
					selectField("languages.java.gradle.runUsing", "Build and run using", "gradle",
						nativeChoice("gradle", "Gradle"), nativeChoice("ide", "GoIDEA")),
					nativeSDKPath("languages.java.gradle.jvm", "Gradle JVM", "Project JDK", "jdk", true),
					selectField("languages.java.gradle.distribution", "Distribution", "wrapper",
						nativeChoice("wrapper", "Gradle wrapper"), nativeChoice("local", "Local installation")),
					nativePath("languages.java.gradle.home", "Gradle home", "Gradle installation directory", true),
					nativePath("languages.java.gradle.userHome", "Gradle user home", "~/.gradle", true),
				},
			}},
		},
		{
			page: settingsPageSpec{
				id: settingsPageMaven, title: "Maven",
				group: "Build, Execution, Deployment", order: 359,
				keywords: []string{
					"delegate build", "javac", "JRE", "runner", "JDK", "JAVA_HOME", "Maven home",
					"mvn", "mvnw", "wrapper", "settings.xml", "user settings", "local repository",
					"m2", "thread count", "threads", "-T", "parallel", "mvnd", "toolchains",
				},
				section: "nativeSettings",
			},
			defaultScope: "project",
			projectOnly:  true,
			sections: []nativeSettingsSection{{
				fields: []nativeSettingsField{
					toggle("languages.java.maven.delegateBuild", "Delegate IDE build/run actions to Maven", false),
					nativeSDKPath("languages.java.maven.runnerJre", "JRE", "Use Project JDK", "jdk", true),
					nativeSDKPath("languages.java.maven.home", "Maven home path", "Maven wrapper or mvn on PATH", "maven", true),
					nativePath("languages.java.maven.userSettings", "User settings file", "~/.m2/settings.xml", false),
					nativePath("languages.java.maven.toolchains", "Toolchains file", "~/.m2/toolchains.xml", false),
					nativePath("languages.java.maven.localRepository", "Local repository", "~/.m2/repository", true),
					text("languages.java.maven.activeProfiles", "Active profiles", "dev,integration", ""),
					text("languages.java.maven.threads", "Thread count", "4, 2C", ""),
					toggle("languages.java.maven.daemon", "Use Maven Daemon (mvnd)", false),
					nativePath("languages.java.maven.mvndHome", "Maven Daemon home", "MVND_HOME or mvnd on PATH", true),
				},
			}},
		},
		{
			page: settingsPageSpec{
				id: settingsPageDebugger, title: "Debugger",
				group: "Build, Execution, Deployment", order: 361,
				keywords: []string{"debug", "breakpoint", "termination"},
				section:  "nativeSettings",
			},
			sections: []nativeSettingsSection{
				{
					fields: []nativeSettingsField{
						toggle("debugger.showOnBreakpoint", "Show debug window on breakpoint hit", true),
						toggle("debugger.focusAppOnBreakpoint", "Focus application on breakpoint", false),
						toggle("debugger.hideOnTermination", "Hide debug window on process termination", false),
					},
				},
				{
					title: "Breakpoints",
					fields: []nativeSettingsField{
						toggle("debugger.breakpointsAnyFile", "Allow placing breakpoints in any file", false),
						toggle("debugger.runToCursorInlay", "Show inlay Run to Cursor popup", true),
					},
				},
			},
		},
		{
			page: settingsPageSpec{
				id: settingsPageDebuggerData, title: "Data Views",
				group: "Build, Execution, Deployment", order: 362,
				keywords: []string{"variables", "inline", "tooltip", "sort"},
				section:  "nativeSettings",
			},
			sections: []nativeSettingsSection{{
				fields: []nativeSettingsField{
					toggle("debugger.inlineValues", "Show values inline", true),
					toggle("debugger.sortValues", "Sort values alphabetically", false),
					toggle("debugger.valueTooltip", "Show value tooltip on mouse hover", true),
					toggle("debugger.valueTooltipOnSelection", "Show value tooltip on code selection", true),
					number("debugger.valueTooltipDelay", "Value tooltip delay", 700, 0, 10_000, 100),
				},
			}},
		},
		{
			page: settingsPageSpec{
				id: settingsPageDebuggerStepping, title: "Stepping",
				group: "Build, Execution, Deployment", order: 363,
				keywords: []string{"step into", "library", "just my code"},
				section:  "nativeSettings",
			},
			sections: []nativeSettingsSection{{
				fields: []nativeSettingsField{
					toggle("debugger.skipLibraryCode", "Do not step into library scripts", true),
				},
			}},
		},
		{
			page: settingsPageSpec{
				id: settingsPagePythonDebugger, title: "Python Debugger",
				group: "Build, Execution, Deployment", order: 364,
				keywords: []string{"debugpy", "python", "gevent", "subprocess"},
				section:  "nativeSettings",
			},
			sections: []nativeSettingsSection{{
				fields: []nativeSettingsField{
					toggle("debugger.python.attachSubprocess",
						"Attach to subprocess automatically while debugging", true),
					toggle("debugger.python.gevent", "Gevent compatible", false),
				},
			}},
		},
		{
			page: settingsPageSpec{
				id: settingsPageRunDebug, title: "Run/Debug",
				group: "Build, Execution, Deployment", order: 365,
				keywords: []string{"run configuration", "rerun", "stop", "terminate", "timeout"},
				section:  "nativeSettings",
			},
			sections: []nativeSettingsSection{{
				fields: []nativeSettingsField{
					toggle("run.confirmRerun", "Confirm rerun with process termination", true),
					toggle("run.confirmTerminate", "Confirm closing a tab with a running process", true),
					toggle("run.showOnStart", "Show Run/Debug window when a process is started", true),
					number("run.terminationTimeoutMs",
						"Time limit for Run/Debug process termination", 3000, 0, 600_000, 500),
				},
			}},
		},
		{
			page: settingsPageSpec{
				id: settingsPagePython, title: "Python", group: "Languages & Frameworks",
				order: 403, keywords: []string{"python", "interpreters", "dependencies", "venv", "ruff"},
				section: "pythonOverview",
			},
		},
		editorSections(settingsPageCodeCompletion, 3, nativeSettingsSection{
			fields: []nativeSettingsField{
				toggle("editor.completion.matchCase", "Match case", true),
				selectField("editor.completion.caseSensitivity", "Case matching", "first",
					nativeChoice("first", "First letter only"), nativeChoice("all", "All letters")),
				toggle("editor.completion.autoInsertSingle", "Automatically insert single suggestions", true),
				toggle("editor.completion.sortAlphabetically", "Sort suggestions alphabetically", false),
				toggle("editor.completion.autoPopup", "Show suggestions as you type", true),
				toggle("editor.completion.insertOnSpecialKeys", "Insert selected suggestion with context keys", false),
				toggle("editor.completion.showDocumentation", "Show the documentation popup", false),
				number("editor.completion.documentationDelay", "Documentation popup delay", 500, 0, 5000, 50),
				toggle("editor.completion.insertParentheses", "Insert parentheses automatically", true),
			},
		}, nativeSettingsSection{
			title: "Parameter Info",
			fields: []nativeSettingsField{
				toggle("editor.completion.parameterInfo", "Show the parameter info popup", true),
				number("editor.completion.parameterInfoDelay", "Parameter info delay", 1000, 0, 5000, 50),
			},
		}),
		{
			page: settingsPageSpec{
				id: settingsPageGutterIcons, title: "Gutter Icons", group: "Editor",
				order: 6, keywords: []string{"run", "bookmark", "inheritors", "overrides", "color preview"},
				section: "gutterIcons",
			},
			sections: []nativeSettingsSection{{
				fields: []nativeSettingsField{
					toggle("editor.showGutterIcons", "Show gutter icons", true),
					list("editor.gutterIconsDisabled", "Disabled gutter groups", "", nil),
				},
			}},
		},
		editorSections(settingsPageInlineCompletion, 7, nativeSettingsSection{
			fields: []nativeSettingsField{
				toggle("editor.inlineCompletion.enabled", "Enable inline completion", true),
				toggle("editor.inlineCompletion.onlyOnPause", "Show only when typing pauses", false),
				number("editor.inlineCompletion.pauseMs", "Pause before requesting", 500, 0, 3000, 50),
				{
					key: "editor.inlineCompletion.disabledLanguages", label: "Show inline completion for",
					kind: nativeSettingsDisabledLanguages, defaultValue: []string{},
				},
			},
		}),
		editorSections(settingsPageRichTextCopy, 8, nativeSettingsSection{
			fields: []nativeSettingsField{
				toggle("editor.general.richTextCopy", "Copy as rich text by default", true),
			},
		}),
		editorSections(settingsPageSmartKeys, 9, nativeSettingsSection{
			fields: []nativeSettingsField{
				toggle("editor.smartKeys.smartHome", "Home moves caret to first non-whitespace character", true),
				toggle("editor.smartKeys.camelHumps", "Use camelCase words", false),
				toggle("editor.smartKeys.pairBrackets", "Insert paired brackets (), [], {}", true),
				toggle("editor.smartKeys.pairQuotes", "Insert pair quote", true),
				toggle("editor.smartKeys.surroundSelection", "Surround selection on typing quote or brace", true),
				toggle("editor.smartKeys.tabOutOfPair", "Jump outside closing bracket or quote with Tab", true),
				selectField("editor.smartKeys.reformatOnPaste", "Reformat on paste", "indentEachLine",
					nativeChoice("none", "None"), nativeChoice("indentBlock", "Indent Block"),
					nativeChoice("indentEachLine", "Indent Each Line")),
				toggle("editor.smartKeys.escapeOnPaste", "Escape text on paste in string literals", true),
			},
		}, nativeSettingsSection{
			title: "Enter",
			fields: []nativeSettingsField{
				toggle("editor.smartKeys.smartIndent", "Smart indent", true),
				toggle("editor.smartKeys.insertDocStub", "Insert documentation comment stub", true),
			},
		}, nativeSettingsSection{
			title: "Backspace",
			fields: []nativeSettingsField{
				selectField("editor.smartKeys.backspaceUnindent", "Unindent", "autoindent",
					nativeChoice("autoindent", "To proper indent position"),
					nativeChoice("nearest", "To nearest indent position"),
					nativeChoice("none", "Disabled")),
			},
		}),
		editorSections(settingsPageStickyLines, 10, nativeSettingsSection{
			fields: []nativeSettingsField{
				toggle("editor.stickyScroll", "Show sticky lines while scrolling", false),
				number("editor.codeEditing.stickyLinesMax", "Maximum number of lines", 5, 1, 20, 1),
			},
		}),
		editorSections(settingsPageCodeEditing, 21, nativeSettingsSection{
			title: "Highlight on Caret Movement",
			fields: []nativeSettingsField{
				toggle("editor.codeEditing.highlightUsages", "Usages of element at caret", true),
				toggle("editor.codeEditing.highlightBraces", "Matched brace", true),
				toggle("editor.codeEditing.highlightSelection", "Occurrences of selected text", true),
			},
		}, nativeSettingsSection{
			title: "Quick Documentation",
			fields: []nativeSettingsField{
				toggle("editor.codeEditing.quickDocOnHover", "Show quick documentation on hover", true),
				number("editor.codeEditing.quickDocDelay", "Tooltip delay", 500, 100, 5000, 50),
			},
		}, nativeSettingsSection{
			title: "Error Highlighting",
			fields: []nativeSettingsField{
				toggle("editor.codeEditing.nextErrorErrorsOnly", "Next error action goes to high priority problems only", true),
			},
		}),
		{
			page: settingsPageSpec{
				id: settingsPageFileEncodings, title: "File Encodings", group: "Editor",
				order: 27, keywords: []string{"encoding", "charset", "utf-8", "bom", "native-to-ascii", "properties"},
				section: "nativeSettings",
			},
			sections: []nativeSettingsSection{
				{
					fields: []nativeSettingsField{
						selectField("editor.encodings.global", "Global Encoding", "utf-8",
							nativeChoices(
								"utf-8", "utf-16le", "utf-16be", "gbk", "gb18030", "big5",
								"shift_jis", "euc-kr", "windows-1252", "iso-8859-1", "koi8-r",
							)...),
						selectField("editor.encodings.project", "Project Encoding", "utf-8",
							nativeChoices(
								"utf-8", "utf-16le", "utf-16be", "gbk", "gb18030", "big5",
								"shift_jis", "euc-kr", "windows-1252", "iso-8859-1", "koi8-r",
							)...),
					},
				},
				{
					title: "Properties Files (*.properties)",
					fields: []nativeSettingsField{
						selectField("editor.encodings.properties", "Default encoding for properties files", "iso-8859-1",
							nativeChoices(
								"utf-8", "utf-16le", "utf-16be", "gbk", "gb18030", "big5",
								"shift_jis", "euc-kr", "windows-1252", "iso-8859-1", "koi8-r",
							)...),
						toggle("editor.encodings.nativeToAscii", "Transparent native-to-ascii conversion", false),
					},
				},
				{
					fields: []nativeSettingsField{
						selectField(utf8BOMSettingID, "Create UTF-8 files", "never",
							nativeChoice("never", "with NO BOM"),
							nativeChoice("always", "with BOM"),
							nativeChoice("windows", "with BOM under Windows, with no BOM otherwise")),
					},
				},
			},
			note: "A charset in .editorconfig takes precedence. Change one file's encoding from the status bar.",
		},
		{
			page: settingsPageSpec{
				id: settingsPageLanguageRegion, title: "Language and Region",
				group: "Appearance & Behavior", order: 0,
				keywords: []string{"locale", "language", "English", "Chinese", "Korean", "French", "Polish", "Portuguese", "Spanish"},
				section:  "nativeSettings",
			},
			defaultScope: "global",
			sections: []nativeSettingsSection{{
				fields: []nativeSettingsField{
					selectField("appearance.locale", "Language", "en",
						nativeChoice("en", "English"),
						nativeChoice("zh", "简体中文 (Chinese, Simplified)"),
						nativeChoice("ko", "한국어 (Korean)"),
						nativeChoice("fr", "Français (French)"),
						nativeChoice("pl", "Polski (Polish)"),
						nativeChoice("pt", "Português (Portuguese)"),
						nativeChoice("es", "Español (Spanish)")),
				},
			}},
			note: "GoIDEA stores the selected language. Native menus and dialogs remain in English until their translations are added.",
		},
		{
			page: settingsPageSpec{
				id: settingsPageHTTPProxy, title: "HTTP Proxy",
				group: "Appearance & Behavior", order: 2,
				keywords: []string{"proxy", "network", "socks", "http_proxy", "no_proxy"},
				section:  "nativeSettings",
			},
			defaultScope: "global",
			sections: []nativeSettingsSection{{
				fields: []nativeSettingsField{
					selectField("proxy.mode", "Proxy mode", "auto",
						nativeChoice("none", "No proxy"),
						nativeChoice("auto", "Auto-detect proxy settings"),
						nativeChoice("manual", "Manual proxy configuration")),
					selectField("proxy.kind", "Proxy type", "http",
						nativeChoice("http", "HTTP"), nativeChoice("socks", "SOCKS")),
					text("proxy.host", "Host name", "", ""),
					number("proxy.port", "Port number", 80, 1, 65535, 1),
					text("proxy.noProxy", "No proxy for", "*.example.com, 192.168.*", ""),
					toggle("proxy.authenticate", "Proxy authentication", false),
					text("proxy.login", "Login", "", ""),
				},
			}},
			note: "Check connection needs proxy_check_connection, which the pinned GoIDEA backend does not expose.",
		},
		{
			page: settingsPageSpec{
				id: settingsPageTerminal, title: "Terminal", group: "Tools", order: 600,
				keywords: []string{"shell", "console", "zsh", "bash", "pty", "font", "cursor"},
				section:  "nativeSettings",
			},
			sections: []nativeSettingsSection{
				{
					title: "Project Settings",
					fields: []nativeSettingsField{
						text("terminal.startDirectory", "Start directory", "Project root", ""),
						{key: "terminal.env", label: "Environment variables", kind: nativeSettingsMap, defaultValue: map[string]string{}},
					},
				},
				{
					title: "Application Settings",
					fields: []nativeSettingsField{
						text("terminal.shellPath", "Shell path", "Login shell", ""),
						text("terminal.tabName", "Default tab name", "Local", "Local"),
						toggle("terminal.audibleBell", "Audible bell", true),
						toggle("terminal.closeOnExit", "Close session when it ends", true),
						toggle("terminal.mouseReporting", "Mouse reporting", true),
						toggle("terminal.copyOnSelection", "Copy to clipboard on selection", false),
						toggle("terminal.pasteOnMiddleClick", "Paste on middle mouse button click", true),
						toggle("terminal.overrideIdeShortcuts", "Override IDE shortcuts", true),
						toggle("terminal.shellIntegration", "Shell integration", true),
						toggle("terminal.highlightHyperlinks", "Highlight hyperlinks", true),
						toggle("terminal.highlightSelection", "Highlight occurrences of selected text", true),
						toggle("terminal.optionAsMeta", "Use Option as Meta key", false),
						toggle("terminal.escapeToEditor", "Move focus to the Editor with Escape", false),
						selectField("terminal.cursorShape", "Cursor shape", "block",
							nativeChoice("block", "Block"), nativeChoice("underline", "Underline"),
							nativeChoice("bar", "Vertical")),
						toggle("terminal.cursorBlink", "Blinking cursor", true),
					},
				},
				{
					title: "Font",
					fields: []nativeSettingsField{
						toggle("terminal.useEditorFont", "Use editor font", true),
						text("terminal.fontFamily", "Font", "JetBrains Mono", "JetBrains Mono"),
						number("terminal.fontSize", "Size", 13, 8, 72, 1),
						number("terminal.lineHeight", "Line height", 1, 0.5, 3, 0.1),
						text("terminal.fallbackFontFamily", "Fallback font", "", ""),
						number("terminal.columnWidth", "Column width", 1, 0.5, 3, 0.1),
					},
				},
			},
		},
		{
			page: settingsPageSpec{
				id: settingsPageActionsOnSave, title: "Actions on Save", group: "Tools", order: 601,
				keywords: []string{"save", "format", "organize imports", "cleanup"},
				section:  "actionsOnSave",
			},
			sections: []nativeSettingsSection{{
				title: "Activate on Save",
				fields: []nativeSettingsField{
					toggle("actionsOnSave.optimizeImports", "Optimize imports", false),
					toggle("actionsOnSave.cleanup", "Run code cleanup", false),
				},
			}},
		},
		{
			page: settingsPageSpec{
				id: settingsPagePythonDeps, title: "Dependencies", group: "Languages & Frameworks",
				order: 4031, keywords: []string{"python interpreter", "virtualenv", "venv", "dependencies"},
				section: "nativeSettings",
			},
			sections: []nativeSettingsSection{{
				fields: []nativeSettingsField{
					nativeSDKPath("languages.python.interpreter", "Python interpreter", "Auto-detect", "python", false),
					toggle("languages.python.activateVirtualenv", "Activate the interpreter's virtualenv in new terminals", true),
				},
			}},
		},
		{
			page: settingsPageSpec{
				id: settingsPagePythonInterpret, title: "All Interpreters", group: "Languages & Frameworks",
				order: 4032, keywords: []string{"python interpreters", "venv", "conda", "pyenv", "poetry", "uv"},
				section: "nativeSettings",
			},
			defaultScope: "global",
			sections: []nativeSettingsSection{{
				fields: []nativeSettingsField{
					func() nativeSettingsField {
						field := nativeSDKPath(
							"languages.python.interpreter", "Project interpreter",
							"Auto · project virtualenv", "python", false,
						)
						field.scope = "project"
						return field
					}(),
					{
						key: "languages.python.interpreters", label: "Added interpreters",
						placeholder: "/path/to/python", kind: nativeSettingsPythonInterpreters,
						defaultValue: []string{},
					},
					list("languages.python.hiddenInterpreters", "Hidden detected interpreters", "/path/to/python", nil),
					toggle("languages.python.hideOtherProjectInterpreters", "Hide interpreters from other projects", false),
					toggle("languages.python.showInterpreterPaths", "Show interpreter paths", true),
				},
			}},
		},
		{
			page: settingsPageSpec{
				id: settingsPagePythonDocstrings, title: "Docstrings", group: "Languages & Frameworks",
				order: 4033, keywords: []string{"docstring", "restructuredtext", "epytext", "google", "numpy"},
				section: "nativeSettings",
			},
			defaultScope: "global",
			sections: []nativeSettingsSection{{
				fields: []nativeSettingsField{
					selectField("editor.docstringFormat", "Documentation string format", "rest",
						nativeChoice("plain", "Plain"), nativeChoice("rest", "reStructuredText"),
						nativeChoice("epytext", "Epytext"), nativeChoice("google", "Google"),
						nativeChoice("numpy", "NumPy")),
				},
			}},
		},
		{
			page: settingsPageSpec{
				id: settingsPageGo, title: "Go", group: "Languages & Frameworks",
				order: 390, keywords: []string{"goroot", "gopath", "go sdk", "gopls"},
				section: "nativeSettings",
			},
			sections: []nativeSettingsSection{
				{title: "GOROOT", fields: []nativeSettingsField{
					nativeSDKPath("languages.go.goroot", "GOROOT", "Go on PATH", "go", true),
				}},
				{title: "GOPATH", fields: []nativeSettingsField{
					nativePath("languages.go.gopath", "Project GOPATH", "go env GOPATH", true),
				}},
			},
		},
		{
			page: settingsPageSpec{
				id: settingsPageGoBuildTags, title: "Build Tags", group: "Languages & Frameworks",
				order: 391, keywords: []string{"goos", "goarch", "cgo", "compiler", "experiments"},
				section: "nativeSettings",
			},
			projectOnly: true,
			sections: []nativeSettingsSection{{
				fields: []nativeSettingsField{
					selectField("languages.go.buildTags.os", "Operating System", "",
						nativeAutomaticChoices("aix", "android", "darwin", "dragonfly", "freebsd", "illumos", "ios", "js", "linux", "netbsd", "openbsd", "plan9", "solaris", "wasip1", "windows")...),
					selectField("languages.go.buildTags.arch", "Architecture", "",
						nativeAutomaticChoices("386", "amd64", "arm", "arm64", "loong64", "mips", "mipsle", "mips64", "mips64le", "ppc64", "ppc64le", "riscv64", "s390x", "wasm")...),
					text("languages.go.buildTags.version", "Go version", "go1.22", ""),
					selectField("languages.go.buildTags.compiler", "Compiler", "",
						nativeChoice("", "Automatic"), nativeChoice("gc", "gc"), nativeChoice("gccgo", "gccgo")),
					selectField("languages.go.buildTags.cgo", "CGO", "",
						nativeChoice("", "Automatic"), nativeChoice("1", "Enabled"), nativeChoice("0", "Disabled")),
					text("languages.go.buildTags.custom", "Custom tags", "integration debug", ""),
					text("languages.go.buildTags.experiments", "Experiments", "loopvar,rangefunc", ""),
				},
			}},
		},
		{
			page: settingsPageSpec{
				id: settingsPageGoModules, title: "Go Modules", group: "Languages & Frameworks",
				order: 392, keywords: []string{"goproxy", "goprivate", "goflags", "gosumdb", "gotoolchain"},
				section: "nativeSettings",
			},
			projectOnly: true,
			sections: []nativeSettingsSection{{
				fields: append([]nativeSettingsField{
					toggle("languages.go.modules.autoVendor", "Enable vendoring support automatically", true),
				}, nativeTextFields("languages.go.modules.env.",
					[]string{"GOPROXY", "GOPRIVATE", "GOFLAGS", "GONOPROXY", "GONOSUMDB", "GOSUMDB", "GOMODCACHE", "GOTOOLCHAIN", "GOWORK"})...),
			}},
		},
		{
			page: settingsPageSpec{
				id: settingsPageNode, title: "Node.js", group: "Languages & Frameworks",
				order: 402, keywords: []string{"node interpreter", "npm", "pnpm", "yarn", "bun", "nvm"},
				section: "nativeSettings",
			},
			sections: []nativeSettingsSection{{
				fields: []nativeSettingsField{
					nativeSDKPath("languages.node.interpreter", "Node interpreter", "node on PATH", "node", false),
					text("languages.node.runTarget", "Run in target", "Default run target", ""),
					selectField("languages.node.packageManager", "Package manager", "",
						nativeChoice("", "Auto"), nativeChoice("npm", "npm"), nativeChoice("pnpm", "pnpm"),
						nativeChoice("yarn", "yarn"), nativeChoice("bun", "bun")),
				},
			}},
		},
		{
			page: settingsPageSpec{
				id: settingsPageMarkdown, title: "Markdown", group: "Languages & Frameworks",
				order: 400, keywords: []string{"markdown", "preview", "font size", "notebook"},
				section: "nativeSettings",
			},
			defaultScope: "global",
			sections: []nativeSettingsSection{{
				title: "Preview",
				fields: []nativeSettingsField{
					number("languages.markdown.previewFontSize", "Preview font size", 14, 8, 72, 1),
				},
			}},
		},
		{
			page: settingsPageSpec{
				id: settingsPagePHP, title: "PHP", group: "Languages & Frameworks",
				order: 4036, keywords: []string{"php interpreter", "composer", "phpstan", "language level", "stubs"},
				section: "nativeSettings",
			},
			sections: []nativeSettingsSection{
				{
					fields: []nativeSettingsField{
						selectField("languages.php.languageLevel", "PHP language level", "",
							nativeAutoChoices("7.4", "8.0", "8.1", "8.2", "8.3", "8.4")...),
						list("languages.php.includePaths", "Include paths", "/path/to/platform", nil),
						list("languages.php.stubs", "Additional stubs", "wordpress", nil),
					},
				},
				{title: "PHPStan", fields: []nativeSettingsField{
					text("languages.php.phpstanConfig", "Configuration file", "Detected in the project root", ""),
				}},
				{title: "CLI Interpreter", fields: []nativeSettingsField{
					text("languages.php.runTarget", "Run in target", "Default run target", ""),
					nativeSDKPath("languages.php.interpreter", "PHP interpreter", "php on PATH", "php", false),
				}},
			},
		},
		{
			page: settingsPageSpec{
				id: settingsPageRuby, title: "Ruby", group: "Languages & Frameworks",
				order: 4038, keywords: []string{"ruby interpreter", "rbenv", "rvm", "bundler", "gems"},
				section: "nativeSettings",
			},
			sections: []nativeSettingsSection{{
				fields: []nativeSettingsField{
					nativeSDKPath("languages.ruby.interpreter", "Ruby interpreter",
						".ruby-version, mise, asdf, or ruby on PATH", "ruby", false),
				},
			}},
		},
		{
			page: settingsPageSpec{
				id: settingsPageRust, title: "Rust", group: "Languages & Frameworks",
				order: 404, keywords: []string{"toolchain", "rustup", "clippy", "cargo check", "rustfmt"},
				section: "nativeSettings",
			},
			sections: []nativeSettingsSection{
				{title: "Toolchain", fields: []nativeSettingsField{
					nativeSDKPath("languages.rust.toolchain", "Toolchain location", "rustup default or PATH", "rust", true),
					nativePath("languages.rust.stdlib", "Standard library", "Detected from the toolchain", true),
				}},
				{title: "External Linters", fields: []nativeSettingsField{
					selectField("languages.rust.checkCommand", "External tool", "check",
						nativeChoice("check", "Cargo Check"), nativeChoice("clippy", "Clippy")),
					toggle("languages.rust.checkOnSave", "Run external linter on save", true),
				}},
				{title: "Rustfmt", fields: []nativeSettingsField{
					text("languages.rust.rustfmtArgs", "Additional arguments", "--edition 2024", ""),
				}},
			},
		},
		{
			page: settingsPageSpec{
				id: settingsPageTypeScript, title: "TypeScript", group: "Languages & Frameworks",
				order: 405, keywords: []string{"typescript version", "tsdk", "tsserver", "node_modules", "vtsls"},
				section: "nativeSettings",
			},
			sections: []nativeSettingsSection{{
				fields: []nativeSettingsField{
					nativeSDKPath("languages.typescript.version", "TypeScript version",
						"bundled, workspace, or a lib directory", "typeScript", true),
				},
			}},
		},
	}
	pageDefs = append(pageDefs, cmakeAndEditorNativeSettingsPages()...)
	pageDefs = append(pageDefs, vcsNativeSettingsPages()...)
	return pageDefs
}

func cmakeAndEditorNativeSettingsPages() []nativeSettingsPageSpec {
	colorSchemeChoices := knownColorSchemeChoices()
	colorSchemeField := func() nativeSettingsField {
		return nativeSelect(editorColorSchemeKey, "Scheme", themeDefaultColorScheme, colorSchemeChoices...)
	}
	colorSchemePage := func(id, title string, order int, keywords []string, sections ...nativeSettingsSection) nativeSettingsPageSpec {
		return nativeSettingsPageSpec{
			page: settingsPageSpec{
				id: id, title: title, group: "Editor", order: order,
				keywords: keywords, section: "nativeSettings",
			},
			defaultScope: "global",
			sections:     sections,
		}
	}
	return []nativeSettingsPageSpec{
		{
			page: settingsPageSpec{
				id: settingsPageCMake, title: "CMake",
				group: "Build, Execution, Deployment", order: 357,
				keywords: []string{
					"profile", "generator", "ninja", "build type", "cmake arguments",
					"build directory", "build arguments", "environment variables",
				},
				section: "nativeSettings",
			},
			defaultScope: "project",
			projectOnly:  true,
			sections: []nativeSettingsSection{{
				title: "CMake profiles",
				fields: []nativeSettingsField{
					nativeCMakeProfiles(),
				},
			}},
			note: "Each profile supports name, buildType, generator, cmakeArgs, buildDir, buildArgs and env.",
		},
		{
			page: settingsPageSpec{
				id: settingsPageCodeStyle, title: "Code Style", group: "Editor", order: 24,
				keywords: []string{
					"indent", "indentation", "tabs", "line separator", "editorconfig",
					"right margin", "scheme", "formatter", "visual guides",
				},
				section: "nativeSettings",
			},
			defaultScope: "project",
			sections: []nativeSettingsSection{{
				title: "Tabs and Indents",
				fields: []nativeSettingsField{
					nativeSelect("editor.codeStyle.lineSeparator", "Line separator", "system",
						nativeChoice("system", "System-Dependent"),
						nativeChoice("lf", "Unix and macOS (\\n)"),
						nativeChoice("crlf", "Windows (\\r\\n)"),
						nativeChoice("cr", "Classic Mac OS (\\r)")),
					nativeNumber("editor.codeStyle.hardWrapAt", "Hard wrap at", 120, 1, 999, 1),
					nativeJSON("editor.codeStyle.visualGuides", "Visual guides", `[80,100]`, []any{}),
					nativeToggle("editor.codeStyle.detectIndents", "Detect and use existing file indents for editing", true),
					nativeToggle("editor.codeStyle.editorconfig", "Enable EditorConfig support", true),
					nativeJSON("editor.codeStyle.languages", "Language code style overrides", `{"java":{"tabSize":4}}`, map[string]any{}),
					nativeList("editor.codeStyle.doNotFormat", "Do not format", "path or glob", nil),
				},
			}, {
				title: "Formatter",
				fields: []nativeSettingsField{
					nativeToggle("editor.codeStyle.formatterTagsEnabled", "Enable formatter markers in comments", true),
					nativeText("editor.codeStyle.formatterOffTag", "Off", "@formatter:off", "@formatter:off"),
					nativeText("editor.codeStyle.formatterOnTag", "On", "@formatter:on", "@formatter:on"),
					nativeToggle("editor.codeStyle.formatterTagsRegexp", "Enable regular expressions in formatter markers", false),
				},
			}},
		},
		{
			page: settingsPageSpec{
				id: settingsPageCodeStyleOther, title: "Other File Types", group: "Editor", order: 1000,
				keywords: []string{"tab size", "indent", "other file types"},
				section:  "nativeSettings",
			},
			defaultScope: "project",
			sections: []nativeSettingsSection{{
				title: "Tabs and Indents",
				fields: []nativeSettingsField{
					nativeInverseToggle("editor.insertSpaces", "Use tab character", true),
					nativeNumber("editor.tabSize", "Tab size", 4, 1, 16, 1),
				},
			}},
		},
		colorSchemePage(settingsPageColorScheme, "Color Scheme", 23,
			[]string{"colors", "syntax highlighting", "theme", "foreground", "background"},
			nativeSettingsSection{fields: []nativeSettingsField{colorSchemeField()}},
		),
		colorSchemePage(settingsPageColorSchemeGen, "General", 1,
			[]string{"caret", "selection", "text", "background", "gutter"},
			nativeSettingsSection{fields: []nativeSettingsField{colorSchemeField()}},
		),
		colorSchemePage(settingsPageColorSchemeLang, "Language Defaults", 2,
			[]string{"semantic highlighting", "rainbow brackets", "keyword", "string", "comment"},
			nativeSettingsSection{fields: []nativeSettingsField{
				colorSchemeField(),
				nativeToggle("editor.semanticHighlighting", "Semantic highlighting", false),
				nativeToggle("editor.rainbowBrackets", "Rainbow brackets", false),
			}},
		),
		colorSchemePage(settingsPageColorSchemeDebug, "Debugger", 3,
			[]string{"breakpoint", "inline values", "watches"},
			nativeSettingsSection{fields: []nativeSettingsField{colorSchemeField()}},
		),
		colorSchemePage(settingsPageColorSchemeCons, "Console Colors", 4,
			[]string{"terminal", "ansi", "console", "background"},
			nativeSettingsSection{fields: []nativeSettingsField{colorSchemeField()}},
		),
	}
}

func nativeChoices(values ...string) []nativeSettingsChoice {
	out := make([]nativeSettingsChoice, len(values))
	for i, value := range values {
		out[i] = nativeChoice(value, value)
	}
	return out
}

func nativeAutomaticChoices(values ...string) []nativeSettingsChoice {
	return append([]nativeSettingsChoice{nativeChoice("", "Automatic")}, nativeChoices(values...)...)
}

func nativeConsoleColors() []nativeSettingsChoice {
	return []nativeSettingsChoice{
		nativeChoice("var(--ansi-red)", "Red"),
		nativeChoice("var(--ansi-yellow)", "Yellow"),
		nativeChoice("var(--ansi-green)", "Green"),
		nativeChoice("var(--ansi-cyan)", "Cyan"),
		nativeChoice("var(--ansi-magenta)", "Magenta"),
	}
}

func nativeAutoChoices(values ...string) []nativeSettingsChoice {
	return append([]nativeSettingsChoice{nativeChoice("", "Auto")}, nativeChoices(values...)...)
}

func nativeTextFields(prefix string, names []string) []nativeSettingsField {
	out := make([]nativeSettingsField, len(names))
	for i, name := range names {
		out[i] = nativeText(prefix+name, name, "Inherit from environment", "")
	}
	return out
}

func nativeSettingsDefinitionsByID() map[string]nativeSettingsPageSpec {
	defs := nativeSettingsDefinitions()
	out := make(map[string]nativeSettingsPageSpec, len(defs))
	for _, def := range defs {
		out[def.page.id] = def
	}
	return out
}

func nativeSettingsPages() []settingsPageSpec {
	defs := nativeSettingsDefinitions()
	pages := make([]settingsPageSpec, 0, len(defs))
	for i := range defs {
		def := &defs[i]
		labels := []string{}
		for _, section := range def.sections {
			if section.title != "" {
				labels = append(labels, section.title)
			}
			for _, field := range section.fields {
				labels = append(labels, field.label)
			}
		}
		def.page.labels = labels
		if def.page.title == "" {
			switch def.page.id {
			case settingsPageAutoImport:
				def.page.title = "Auto Import"
			case settingsPageCodeCompletion:
				def.page.title = "Code Completion"
			case settingsPageRichTextCopy:
				def.page.title = "Rich-Text Copy"
			case settingsPageSmartKeys:
				def.page.title = "Smart Keys"
			case settingsPageInlineCompletion:
				def.page.title = "Inline Completion"
			case settingsPageStickyLines:
				def.page.title = "Sticky Lines"
			case settingsPageCodeEditing:
				def.page.title = "Code Editing"
			}
		}
		pages = append(pages, def.page)
	}
	return pages
}

func nativeSettingsDefinition(page string) (nativeSettingsPageSpec, bool) {
	def, ok := nativeSettingsDefinitionsByID()[page]
	return def, ok
}

func nativeSettingsPageLoadable(page string) bool {
	def, ok := nativeSettingsDefinition(page)
	return ok && len(def.sections) > 0
}

type nativeSettingsPageState struct {
	loading bool
	loaded  bool
	session bool
	scope   string
	root    string
	applied map[string]any
	draft   map[string]any
	aux     map[string]string
	err     string
	loadGen uint64
}

type nativeSettingsCatalogState struct {
	pages  map[string]nativeSettingsPageState
	saving bool
}

func nativeSettingsCloneValues(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		switch typed := value.(type) {
		case []string:
			out[key] = cloneStrings(typed)
		case map[string]string:
			copied := make(map[string]string, len(typed))
			for name, entry := range typed {
				copied[name] = entry
			}
			out[key] = copied
		case []any, map[string]any:
			out[key] = cloneNativeJSONValue(typed)
		default:
			out[key] = typed
		}
	}
	return out
}

func nativeSettingsClonePage(page nativeSettingsPageState) nativeSettingsPageState {
	page.applied = nativeSettingsCloneValues(page.applied)
	page.draft = nativeSettingsCloneValues(page.draft)
	if page.aux != nil {
		aux := make(map[string]string, len(page.aux))
		for key, value := range page.aux {
			aux[key] = value
		}
		page.aux = aux
	}
	return page
}

func nativeSettingsPageScope(m *Model, def nativeSettingsPageSpec) string {
	scopes := nativeSettingsPageScopes(m, def)
	if len(scopes) == 1 {
		return scopes[0]
	}
	return "mixed"
}

func nativeSettingsFieldScope(field nativeSettingsField, fallback string) string {
	if field.scope != "" {
		return field.scope
	}
	return fallback
}

func nativeSettingsPageScopes(m *Model, def nativeSettingsPageSpec) []string {
	if m.settingsDraft.newProjects {
		return []string{"global"}
	}
	fallback := def.defaultScope
	if fallback == "" {
		fallback = m.settingsBackendScope()
	}
	found := map[string]bool{}
	for _, section := range def.sections {
		for _, field := range section.fields {
			scope := nativeSettingsFieldScope(field, fallback)
			if scope == "project" && m.Root() == "" {
				continue
			}
			found[scope] = true
		}
	}
	if len(found) == 0 {
		found[fallback] = true
	}
	scopes := make([]string, 0, len(found))
	for scope := range found {
		scopes = append(scopes, scope)
	}
	sort.Strings(scopes)
	return scopes
}

func nativeSettingsScopesIncludeProject(scopes []string) bool {
	for _, scope := range scopes {
		if scope == "project" {
			return true
		}
	}
	return false
}

func flattenNativeSettings(prefix string, value any, out map[string]any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			next := key
			if prefix != "" {
				next = prefix + "." + key
			}
			flattenNativeSettings(next, child, out)
		}
	default:
		out[prefix] = typed
	}
}

func nativeSettingsPageValues(raw json.RawMessage, def nativeSettingsPageSpec) (map[string]any, error) {
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, err
	}
	fallback := def.defaultScope
	if fallback == "" {
		fallback = "global"
	}
	roots := map[string]map[string]any{}
	for _, section := range def.sections {
		for _, field := range section.fields {
			roots[nativeSettingsFieldScope(field, fallback)] = root
		}
	}
	return nativeSettingsPageValuesFromRoots(roots, def), nil
}

func nativeSettingsPageValuesFromRoots(roots map[string]map[string]any, def nativeSettingsPageSpec) map[string]any {
	fallback := def.defaultScope
	if fallback == "" {
		if len(roots) == 1 {
			for scope := range roots {
				fallback = scope
			}
		} else {
			fallback = "global"
		}
	}
	flatByScope := map[string]map[string]any{}
	for scope, root := range roots {
		flat := map[string]any{}
		flattenNativeSettings("", root, flat)
		flatByScope[scope] = flat
	}
	values := map[string]any{}
	for _, section := range def.sections {
		for _, field := range section.fields {
			scope := nativeSettingsFieldScope(field, fallback)
			root := roots[scope]
			if field.kind == nativeSettingsMap {
				value := nativeNestedSetting(root, strings.Split(field.key, ".")...)
				if value == nil {
					value = nativeSettingsDefault(field)
				}
				values[field.key] = nativeSettingsNormalizeValue(field, value)
				continue
			}
			if field.kind == nativeSettingsJSON || field.kind == nativeSettingsCMakeProfiles {
				value := nativeNestedSetting(root, strings.Split(field.key, ".")...)
				values[field.key] = nativeSettingsNormalizeValue(field, value)
				continue
			}
			flat := flatByScope[scope]
			value, ok := flat[field.key]
			if !ok {
				value = nativeSettingsDefault(field)
			}
			values[field.key] = nativeSettingsNormalizeValue(field, value)
		}
	}
	return values
}

func nativeSettingsDefault(field nativeSettingsField) any {
	switch value := field.defaultValue.(type) {
	case []string:
		return cloneStrings(value)
	case map[string]string:
		copied := make(map[string]string, len(value))
		for key, entry := range value {
			copied[key] = entry
		}
		return copied
	case []any, map[string]any:
		return cloneNativeJSONValue(value)
	default:
		return value
	}
}

func nativeSettingsNormalizeValue(field nativeSettingsField, value any) any {
	switch field.kind {
	case nativeSettingsToggle:
		if typed, ok := value.(bool); ok {
			return typed
		}
		return field.defaultValue
	case nativeSettingsNumber:
		switch typed := value.(type) {
		case float64:
			return typed
		case float32:
			return float64(typed)
		case int:
			return float64(typed)
		case int64:
			return float64(typed)
		case json.Number:
			number, _ := typed.Float64()
			return number
		}
		return field.defaultValue
	case nativeSettingsList, nativeSettingsDisabledLanguages, nativeSettingsPythonInterpreters:
		return nativeSettingsStringList(value)
	case nativeSettingsMap:
		out := map[string]string{}
		switch typed := value.(type) {
		case map[string]any:
			for key, entry := range typed {
				if text, ok := entry.(string); ok {
					out[key] = text
				}
			}
		case map[string]string:
			for key, entry := range typed {
				out[key] = entry
			}
		}
		return out
	case nativeSettingsJSON, nativeSettingsCMakeProfiles:
		switch typed := value.(type) {
		case nil:
			return cloneNativeJSONValue(field.defaultValue)
		case []any, map[string]any:
			return cloneNativeJSONValue(typed)
		default:
			encoded, err := json.Marshal(typed)
			if err != nil {
				return cloneNativeJSONValue(field.defaultValue)
			}
			var decoded any
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				return cloneNativeJSONValue(field.defaultValue)
			}
			return decoded
		}
	default:
		if typed, ok := value.(string); ok {
			return typed
		}
		return field.defaultValue
	}
}

func cloneNativeJSONValue(value any) any {
	encoded, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var decoded any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return value
	}
	return decoded
}

func nativeSettingsStringList(value any) []string {
	switch typed := value.(type) {
	case []string:
		return cloneStrings(typed)
	case []any:
		out := make([]string, 0, len(typed))
		for _, entry := range typed {
			if text, ok := entry.(string); ok {
				out = append(out, text)
			}
		}
		return out
	default:
		return []string{}
	}
}

func (m *Model) loadNativeSettingsPage(page string) {
	def, ok := nativeSettingsDefinition(page)
	if !ok || len(def.sections) == 0 {
		return
	}
	if m.nativeSettings.pages == nil {
		m.nativeSettings.pages = map[string]nativeSettingsPageState{}
	}
	state := m.nativeSettings.pages[page]
	scope := nativeSettingsPageScope(m, def)
	root := m.Root()
	if state.session && (state.loading || state.loaded) && state.scope == scope && state.root == root {
		return
	}
	state.session = true
	state.loading = true
	state.loaded = false
	state.scope = scope
	state.root = root
	state.err = ""
	state.loadGen++
	generation := state.loadGen
	m.nativeSettings.pages[page] = state
	if m.mgr == nil && m.invokeHook == nil {
		state.loading = false
		state.err = "The GoIDEA backend is not connected"
		m.nativeSettings.pages[page] = state
		m.invalidate()
		return
	}
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(modelContext(m), 20*time.Second)
		defer cancel()
		scopes := nativeSettingsPageScopes(m, def)
		roots := map[string]map[string]any{}
		var err error
		for _, requestedScope := range scopes {
			var raw json.RawMessage
			raw, err = m.invokeSettingsScope(ctx, requestedScope)
			if err != nil {
				break
			}
			var root map[string]any
			err = json.Unmarshal(raw, &root)
			if err != nil {
				break
			}
			roots[requestedScope] = root
		}
		values := nativeSettingsPageValuesFromRoots(roots, def)
		m.post(func() {
			current := m.nativeSettings.pages[page]
			if current.loadGen != generation || current.scope != nativeSettingsPageScope(m, def) ||
				((def.projectOnly || nativeSettingsScopesIncludeProject(scopes)) && m.Root() != root) {
				return
			}
			current.loading = false
			current.loaded = err == nil
			current.aux = map[string]string{}
			if err != nil {
				current.err = "Could not read " + def.page.title + ": " + err.Error()
				m.nativeSettings.pages[page] = current
				m.invalidate()
				return
			}
			current.applied = nativeSettingsCloneValues(values)
			current.draft = nativeSettingsCloneValues(values)
			if page == settingsPageActionsOnSave {
				if format, ok := nativeNestedSetting(roots["global"], "editor", "formatOnSave").(bool); ok {
					m.settingsDraft.reformatOnSave = format
				}
			}
			m.nativeSettings.pages[page] = current
			if page == settingsPageAgentMCPServer {
				m.trimAgentRuns()
			}
			switch page {
			case settingsPageDebugger, settingsPageDebuggerData,
				settingsPageDebuggerStepping, settingsPagePythonDebugger:
				m.applyDebuggerRuntimeSettings(map[string]nativeSettingsPageState{page: current})
			case settingsPageRunDebug:
				m.applyRunRuntimeSettings(map[string]nativeSettingsPageState{page: current})
			}
			m.invalidate()
		})
	}()
}

func nativeNestedSetting(root map[string]any, path ...string) any {
	var current any = root
	for _, part := range path {
		values, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = values[part]
	}
	return current
}

func nativeSettingsValue(m *Model, page, key string) any {
	if state, ok := m.nativeSettings.pages[page]; ok {
		return state.draft[key]
	}
	if def, ok := nativeSettingsDefinition(page); ok {
		for _, section := range def.sections {
			for _, field := range section.fields {
				if field.key == key {
					return nativeSettingsDefault(field)
				}
			}
		}
	}
	return nil
}

func (m *Model) setNativeSettingsValue(page, key string, value any) {
	state := m.nativeSettings.pages[page]
	if state.draft == nil {
		state.draft = map[string]any{}
	}
	state.draft[key] = value
	m.nativeSettings.pages[page] = state
	if key == "languages.python.interpreter" {
		for _, other := range []string{settingsPagePythonDeps, settingsPagePythonInterpret} {
			if other == page {
				continue
			}
			otherState := m.nativeSettings.pages[other]
			if !otherState.loaded {
				continue
			}
			if otherState.draft == nil {
				otherState.draft = map[string]any{}
			}
			otherState.draft[key] = value
			m.nativeSettings.pages[other] = otherState
		}
	}
	m.invalidate()
}

func nativeSettingsBool(m *Model, page, key string) bool {
	value, _ := nativeSettingsValue(m, page, key).(bool)
	return value
}

func nativeSettingsString(m *Model, page, key string) string {
	value, _ := nativeSettingsValue(m, page, key).(string)
	return value
}

func nativeSettingsNumberValue(m *Model, page, key string) float64 {
	switch value := nativeSettingsValue(m, page, key).(type) {
	case float64:
		return value
	case int:
		return float64(value)
	}
	return 0
}

func nativeSettingsSameValue(a, b any) bool {
	switch left := a.(type) {
	case []string:
		right, ok := b.([]string)
		return ok && sameStrings(left, right)
	case map[string]string:
		right, ok := b.(map[string]string)
		if !ok || len(left) != len(right) {
			return false
		}
		for key, value := range left {
			if right[key] != value {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(a, b)
	}
}

func (m *Model) nativeSettingsDirty(page string) bool {
	state, ok := m.nativeSettings.pages[page]
	if !ok || !state.loaded {
		return false
	}
	def, ok := nativeSettingsDefinition(page)
	if !ok {
		return false
	}
	for _, section := range def.sections {
		for _, field := range section.fields {
			if !nativeSettingsSameValue(state.applied[field.key], state.draft[field.key]) {
				return true
			}
		}
	}
	if page == settingsPageActionsOnSave {
		return false
	}
	return false
}

func (m *Model) resetNativeSettingsPage(page string) {
	def, ok := nativeSettingsDefinition(page)
	if !ok {
		return
	}
	state := m.nativeSettings.pages[page]
	if state.draft == nil {
		state.draft = map[string]any{}
	}
	for _, section := range def.sections {
		for _, field := range section.fields {
			state.draft[field.key] = nativeSettingsDefault(field)
		}
	}
	state.aux = map[string]string{}
	m.nativeSettings.pages[page] = state
	if page == settingsPageActionsOnSave {
		m.settingsDraft.reformatOnSave = false
	}
	m.invalidate()
}

func (m *Model) pendingNativeSettingsWrites() []settingsBackendWrite {
	defs := nativeSettingsDefinitionsByID()
	pages := make([]string, 0, len(m.nativeSettings.pages))
	for page := range m.nativeSettings.pages {
		pages = append(pages, page)
	}
	sort.Strings(pages)
	var writes []settingsBackendWrite
	for _, page := range pages {
		state, ok := m.nativeSettings.pages[page]
		def, defined := defs[page]
		if !ok || !defined || !state.loaded {
			continue
		}
		fallbackScope := def.defaultScope
		if fallbackScope == "" {
			fallbackScope = m.settingsBackendScope()
		}
		if m.settingsDraft.newProjects {
			fallbackScope = "global"
		}
		for _, section := range def.sections {
			for _, field := range section.fields {
				before := state.applied[field.key]
				after := state.draft[field.key]
				if !nativeSettingsSameValue(before, after) {
					scope := nativeSettingsFieldScope(field, fallbackScope)
					if m.settingsDraft.newProjects {
						scope = "global"
					}
					writes = append(writes, settingsBackendWrite{
						scope, field.key, after,
					})
				}
			}
		}
	}
	return writes
}

func (m *Model) settingsNativeBody(c *ui.Context, oi oiColors, page string) {
	def, ok := nativeSettingsDefinition(page)
	if !ok {
		return
	}
	if page == settingsPagePython {
		settingsSectionTitle(c, oi, "Python")
		ui.Text(c, "Python interpreters, dependencies and docstring formatting are configured on the child pages.").
			FontSize(12).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
		return
	}
	if def.projectOnly && m.Root() == "" {
		ui.Text(c, "Open a project to configure "+def.page.title+".").
			FontSize(12).TextColor(oi.textMuted).SingleLine()
		return
	}
	state := m.nativeSettings.pages[page]
	settingsBackendScopeText(c, oi, state.scope)
	settingsBackendStatus(c, oi, state.loading, state.err, def.page.title, func() {
		m.nativeSettings.pages[page] = nativeSettingsPageState{}
		m.loadNativeSettingsPage(page)
	})
	if !state.loaded {
		return
	}
	m.ensureNativeLanguageSDKs(page, false)
	if page == settingsPageActionsOnSave {
		reformat := m.settingsDraft.reformatOnSave
		if ui.Checkbox(c, &reformat, "Reformat code").Changed() {
			m.settingsDraft.reformatOnSave = reformat
			m.invalidate()
		}
		ui.Text(c, "Formats the whole file with the language server's formatter.").
			FontSize(11).TextColor(oi.textMuted).PaddingStart(20).SingleLine().Ellipsis("…")
	}
	if !m.settingsVCSNativeBody(c, oi, page) {
		for _, section := range def.sections {
			if section.title != "" {
				settingsSectionTitle(c, oi, section.title)
			}
			for _, field := range section.fields {
				m.renderNativeSettingsField(c, oi, page, field)
			}
		}
	}
	if page == settingsPagePythonDeps {
		m.renderPythonPackages(c, oi)
	}
	if page == settingsPagePHP {
		m.renderPHPStatus(c, oi)
	}
	if page == settingsPageAgentMCPServer {
		m.renderMCPServerEndpoint(c, oi)
	}
	if def.note != "" {
		ui.Text(c, def.note).FontSize(11).TextColor(oi.textMuted).SingleLine().Ellipsis("…")
	}
}

func (m *Model) renderNativeSettingsField(c *ui.Context, oi oiColors, page string, field nativeSettingsField) {
	switch field.kind {
	case nativeSettingsToggle:
		value := nativeSettingsBool(m, page, field.key)
		if field.invert {
			value = !value
		}
		if ui.Checkbox(c, &value, field.label).Changed() {
			if field.invert {
				value = !value
			}
			m.setNativeSettingsValue(page, field.key, value)
		}
	case nativeSettingsText:
		ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
			settingsFormLabel(c, oi, field.label, 230)
			value := nativeSettingsString(m, page, field.key)
			input := ui.TextInput(c, &value).Grow(1).Placeholder(field.placeholder)
			if input.Changed() {
				m.setNativeSettingsValue(page, field.key, value)
			}
		})
	case nativeSettingsNumber:
		ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
			settingsFormLabel(c, oi, field.label, 230)
			value := nativeSettingsNumberValue(m, page, field.key)
			input := ui.NumberInput(c, &value, field.min, field.max, field.step).Width(120)
			if input.Changed() {
				m.setNativeSettingsValue(page, field.key, value)
			}
		})
	case nativeSettingsSelect:
		if field.key == "languages.node.packageManager" {
			m.renderNativePackageManager(c, oi, page, field)
			break
		}
		if field.key == editorColorSchemeKey {
			field.choices = m.colorSchemeChoices(field.choices)
		}
		ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
			settingsFormLabel(c, oi, field.label, 230)
			value := nativeSettingsString(m, page, field.key)
			label := value
			labels := make([]string, len(field.choices))
			for i, choice := range field.choices {
				labels[i] = choice.label
				if choice.value == value {
					label = choice.label
				}
			}
			selector := ui.Select(c, &label, labels).Grow(1)
			if selector.Changed() {
				for _, choice := range field.choices {
					if choice.label == label {
						m.setNativeSettingsValue(page, field.key, choice.value)
						break
					}
				}
			}
		})
	case nativeSettingsList:
		m.renderNativeSettingsList(c, oi, page, field)
	case nativeSettingsMap:
		m.renderNativeSettingsMap(c, oi, page, field)
	case nativeSettingsDisabledLanguages:
		m.renderNativeSettingsLanguages(c, oi, page, field)
	case nativeSettingsPath:
		m.renderNativePath(c, page, field)
	case nativeSettingsSDKPath:
		m.renderNativeSDKPath(c, oi, page, field)
	case nativeSettingsPythonInterpreters:
		m.renderPythonInterpreters(c, oi, field)
	case nativeSettingsJSON:
		ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).Children(func() {
			settingsFormLabel(c, oi, field.label, 230)
			text := nativeSettingsJSONText(nativeSettingsValue(m, page, field.key))
			input := ui.TextInput(c, &text).Grow(1).Placeholder(field.placeholder)
			if input.Changed() {
				var parsed any
				if err := json.Unmarshal([]byte(text), &parsed); err == nil {
					m.setNativeSettingsValue(page, field.key, parsed)
				}
			}
		})
	case nativeSettingsCMakeProfiles:
		m.renderNativeCMakeProfiles(c, oi, page, field)
	}
	if field.hint != "" {
		ui.Text(c, field.hint).FontSize(11).TextColor(oi.textMuted).
			PaddingStart(20).SingleLine().Ellipsis("…")
	}
}

func nativeSettingsJSONText(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func nativeCMakeProfileList(value any) []map[string]any {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	profiles := make([]map[string]any, 0, len(items))
	for _, item := range items {
		profile, ok := item.(map[string]any)
		if !ok {
			continue
		}
		profiles = append(profiles, nativeCMakeProfile(profile))
	}
	return profiles
}

func nativeCMakeProfile(profile map[string]any) map[string]any {
	defaultStrings := map[string]string{
		"name": "Debug", "buildType": "Debug", "generator": "", "buildDir": "",
	}
	out := map[string]any{
		"name": defaultStrings["name"], "buildType": defaultStrings["buildType"],
		"generator": defaultStrings["generator"], "cmakeArgs": []any{},
		"buildDir": defaultStrings["buildDir"], "buildArgs": []any{},
		"env": map[string]any{},
	}
	for key, value := range profile {
		out[key] = cloneNativeJSONValue(value)
	}
	for _, key := range []string{"name", "buildType", "generator", "buildDir"} {
		if _, ok := out[key].(string); !ok {
			out[key] = defaultStrings[key]
		}
	}
	if _, ok := out["cmakeArgs"].([]any); !ok {
		out["cmakeArgs"] = []any{}
	}
	if _, ok := out["buildArgs"].([]any); !ok {
		out["buildArgs"] = []any{}
	}
	if _, ok := out["env"].(map[string]any); !ok {
		out["env"] = map[string]any{}
	}
	return out
}

func (m *Model) renderNativeCMakeProfiles(c *ui.Context, oi oiColors, page string, field nativeSettingsField) {
	state := m.nativeSettings.pages[page]
	if state.aux == nil {
		state.aux = map[string]string{}
	}
	profiles := nativeCMakeProfileList(nativeSettingsValue(m, page, field.key))
	selectedKey := field.key + ".selected"
	selected := 0
	if text := state.aux[selectedKey]; text != "" {
		if index := strings.IndexByte(text, '\x00'); index >= 0 {
			text = text[:index]
		}
		if parsed, err := strconv.Atoi(text); err == nil {
			selected = parsed
		}
	}
	if selected >= len(profiles) {
		selected = max(0, len(profiles)-1)
	}
	setProfiles := func(next []map[string]any) {
		values := make([]any, len(next))
		for i := range next {
			values[i] = next[i]
			if i == selected {
				state.aux[selectedKey] = strconv.Itoa(i)
			}
		}
		m.nativeSettings.pages[page] = state
		m.setNativeSettingsValue(page, field.key, values)
	}
	ui.Text(c, field.label).FontSize(12).Bold().TextColor(oi.text).PaddingStart(20).SingleLine()
	ui.Row(c).Gap(6).AlignItems(ui.Center).PaddingStart(20).Children(func() {
		ui.Button(c, "Add").Width(56).OnClick(func() {
			next := append([]map[string]any{}, profiles...)
			added := nativeCMakeProfile(map[string]any{
				"name": fmt.Sprintf("CMake profile %d", len(next)+1),
			})
			next = append(next, added)
			selected = len(next) - 1
			setProfiles(next)
		})
		ui.Button(c, "Remove").Width(76).Disabled(len(profiles) == 0).OnClick(func() {
			if len(profiles) == 0 {
				return
			}
			next := append([]map[string]any{}, profiles[:selected]...)
			next = append(next, profiles[selected+1:]...)
			if selected >= len(next) {
				selected = max(0, len(next)-1)
			}
			setProfiles(next)
		})
	})
	if len(profiles) == 0 {
		ui.Text(c, "No CMake profiles. Click Add to add one.").
			FontSize(11).TextColor(oi.textMuted).PaddingStart(20).SingleLine()
		return
	}
	for i := range profiles {
		i := i
		label := profiles[i]["name"].(string)
		ui.Row(c).Height(26).PaddingStart(20).Children(func() {
			button := ui.Button(c, label).Grow(1)
			if i == selected {
				button.Background(oi.selected)
			}
			button.OnClick(func() {
				selected = i
				state.aux[selectedKey] = strconv.Itoa(i)
				m.nativeSettings.pages[page] = state
				m.invalidate()
			})
		})
	}
	profile := profiles[selected]
	update := func(key string, value any) {
		next := append([]map[string]any{}, profiles...)
		current := nativeCMakeProfile(next[selected])
		current[key] = cloneNativeJSONValue(value)
		next[selected] = current
		setProfiles(next)
	}
	for _, key := range []string{"name", "buildType", "generator", "buildDir"} {
		key := key
		label := map[string]string{
			"name": "Name", "buildType": "Build type", "generator": "Generator", "buildDir": "Build directory",
		}[key]
		m.renderCMakeProfileText(c, oi, label, profile[key].(string), func(value string) {
			update(key, value)
		})
	}
	for _, key := range []string{"cmakeArgs", "buildArgs", "env"} {
		key := key
		label := map[string]string{
			"cmakeArgs": "CMake arguments", "buildArgs": "Build arguments", "env": "Environment variables",
		}[key]
		m.renderCMakeProfileText(c, oi, label, nativeSettingsJSONText(profile[key]), func(value string) {
			var parsed any
			if err := json.Unmarshal([]byte(value), &parsed); err == nil {
				update(key, parsed)
			}
		})
	}
	ui.Text(c, "Quote arguments containing spaces. Build arguments go to cmake --build.").
		FontSize(11).TextColor(oi.textMuted).PaddingStart(20).SingleLine().Ellipsis("…")
}

func (m *Model) renderCMakeProfileText(c *ui.Context, oi oiColors, label, value string, changed func(string)) {
	ui.Row(c).MinHeight(32).Gap(8).AlignItems(ui.Center).PaddingStart(40).Children(func() {
		settingsFormLabel(c, oi, label, 190)
		input := ui.TextInput(c, &value).Grow(1)
		if input.Changed() {
			changed(value)
		}
	})
}

func nativeSettingsListValue(m *Model, page, key string) []string {
	return nativeSettingsStringList(nativeSettingsValue(m, page, key))
}

func (m *Model) renderNativeSettingsList(c *ui.Context, oi oiColors, page string, field nativeSettingsField) {
	ui.Text(c, field.label).FontSize(12).Bold().TextColor(oi.text).PaddingStart(20).SingleLine()
	values := nativeSettingsListValue(m, page, field.key)
	if len(values) == 0 {
		ui.Text(c, "No entries").FontSize(11).TextColor(oi.textMuted).PaddingStart(20).SingleLine()
	}
	for i := range values {
		i := i
		ui.Row(c).MinHeight(30).Gap(6).AlignItems(ui.Center).PaddingStart(20).Children(func() {
			value := values[i]
			input := ui.TextInput(c, &value).Grow(1).Placeholder(field.placeholder)
			if input.Changed() {
				updated := nativeSettingsListValue(m, page, field.key)
				if i < len(updated) {
					updated[i] = value
					m.setNativeSettingsValue(page, field.key, updated)
				}
			}
			ui.Button(c, "Remove").Width(72).OnClick(func() {
				updated := nativeSettingsListValue(m, page, field.key)
				if i < len(updated) {
					m.setNativeSettingsValue(page, field.key, append(updated[:i:i], updated[i+1:]...))
				}
			})
		})
	}
	state := m.nativeSettings.pages[page]
	if state.aux == nil {
		state.aux = map[string]string{}
	}
	inputKey := field.key + ".draft"
	ui.Row(c).MinHeight(30).Gap(6).AlignItems(ui.Center).PaddingStart(20).Children(func() {
		draft := state.aux[inputKey]
		input := ui.TextInput(c, &draft).Grow(1).Placeholder(field.placeholder)
		if input.Changed() {
			state.aux[inputKey] = draft
			m.nativeSettings.pages[page] = state
		}
		ui.Button(c, "Add").Width(56).OnClick(func() {
			state := m.nativeSettings.pages[page]
			value := strings.TrimSpace(state.aux[inputKey])
			if value == "" || containsStringFold(nativeSettingsListValue(m, page, field.key), value) {
				return
			}
			values := nativeSettingsListValue(m, page, field.key)
			values = append(values, value)
			delete(state.aux, inputKey)
			m.nativeSettings.pages[page] = state
			m.setNativeSettingsValue(page, field.key, values)
		})
	})
}

func (m *Model) renderNativeSettingsMap(c *ui.Context, oi oiColors, page string, field nativeSettingsField) {
	ui.Text(c, field.label).FontSize(12).Bold().TextColor(oi.text).PaddingStart(20).SingleLine()
	values, _ := nativeSettingsValue(m, page, field.key).(map[string]string)
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		ui.Text(c, "No variables").FontSize(11).TextColor(oi.textMuted).PaddingStart(20).SingleLine()
	}
	for _, key := range keys {
		key := key
		ui.Row(c).MinHeight(30).Gap(6).AlignItems(ui.Center).PaddingStart(20).Children(func() {
			name := key
			ui.TextInput(c, &name).Width(180)
			value := values[key]
			input := ui.TextInput(c, &value).Grow(1)
			if input.Changed() {
				current, _ := nativeSettingsValue(m, page, field.key).(map[string]string)
				current[key] = value
				m.setNativeSettingsValue(page, field.key, current)
			}
			ui.Button(c, "Remove").Width(72).OnClick(func() {
				current, _ := nativeSettingsValue(m, page, field.key).(map[string]string)
				delete(current, key)
				m.setNativeSettingsValue(page, field.key, current)
			})
		})
	}
	state := m.nativeSettings.pages[page]
	if state.aux == nil {
		state.aux = map[string]string{}
	}
	nameKey, valueKey := field.key+".name", field.key+".value"
	ui.Row(c).MinHeight(30).Gap(6).AlignItems(ui.Center).PaddingStart(20).Children(func() {
		name, value := state.aux[nameKey], state.aux[valueKey]
		nameInput := ui.TextInput(c, &name).Width(180).Placeholder("NAME")
		valueInput := ui.TextInput(c, &value).Grow(1).Placeholder("value")
		if nameInput.Changed() || valueInput.Changed() {
			state.aux[nameKey], state.aux[valueKey] = name, value
			m.nativeSettings.pages[page] = state
		}
		ui.Button(c, "Add").Width(56).OnClick(func() {
			state := m.nativeSettings.pages[page]
			name := strings.TrimSpace(state.aux[nameKey])
			if name == "" {
				return
			}
			current, _ := nativeSettingsValue(m, page, field.key).(map[string]string)
			current[name] = state.aux[valueKey]
			delete(state.aux, nameKey)
			delete(state.aux, valueKey)
			m.nativeSettings.pages[page] = state
			m.setNativeSettingsValue(page, field.key, current)
		})
	})
}

func (m *Model) renderNativeSettingsLanguages(c *ui.Context, oi oiColors, page string, field nativeSettingsField) {
	ui.Text(c, field.label).FontSize(12).Bold().TextColor(oi.text).PaddingStart(20).SingleLine()
	disabled := nativeSettingsListValue(m, page, field.key)
	var languages []string
	for _, language := range editor.Languages() {
		id := strings.ToLower(strings.TrimSpace(language.Name))
		if id != "" && id != "plaintext" && id != "log" && id != "diff" {
			languages = append(languages, language.Name+"\x00"+id)
		}
	}
	sort.Strings(languages)
	for _, entry := range languages {
		name, languageID, _ := strings.Cut(entry, "\x00")
		id := languageID
		enabled := !containsStringFold(disabled, id)
		if ui.Checkbox(c, &enabled, name).Changed() {
			current := nativeSettingsListValue(m, page, field.key)
			if enabled {
				for i, value := range current {
					if value == id {
						current = append(current[:i:i], current[i+1:]...)
						break
					}
				}
			} else if !containsStringFold(current, id) {
				current = append(current, id)
			}
			m.setNativeSettingsValue(page, field.key, current)
		}
	}
}

func (m *Model) nativeGutterBody(c *ui.Context, oi oiColors) {
	page := settingsPageGutterIcons
	state := m.nativeSettings.pages[page]
	settingsBackendScopeText(c, oi, state.scope)
	settingsBackendStatus(c, oi, state.loading, state.err, "Gutter Icons", func() {
		m.nativeSettings.pages[page] = nativeSettingsPageState{}
		m.loadNativeSettingsPage(page)
	})
	if !state.loaded {
		return
	}
	master := nativeSettingsBool(m, page, "editor.showGutterIcons")
	if ui.Checkbox(c, &master, "Show gutter icons").Changed() {
		m.setNativeSettingsValue(page, "editor.showGutterIcons", master)
	}
	groups := []nativeSettingsChoice{
		nativeChoice("run", "Run configurations and tests"),
		nativeChoice("bookmarks", "Bookmarks"),
		nativeChoice("inheritors", "Implemented and overridden declarations"),
		nativeChoice("supers", "Implementing and overriding methods"),
		nativeChoice("colors", "Color preview"),
	}
	disabled := nativeSettingsListValue(m, page, "editor.gutterIconsDisabled")
	for _, group := range groups {
		group := group
		enabled := !containsStringFold(disabled, group.value)
		check := ui.Checkbox(c, &enabled, group.label).PaddingStart(20).Disabled(!master)
		if check.Changed() {
			current := nativeSettingsListValue(m, page, "editor.gutterIconsDisabled")
			if enabled {
				for i, value := range current {
					if value == group.value {
						current = append(current[:i:i], current[i+1:]...)
						break
					}
				}
			} else if !containsStringFold(current, group.value) {
				current = append(current, group.value)
			}
			m.setNativeSettingsValue(page, "editor.gutterIconsDisabled", current)
		}
	}
}

func nativeSettingsPageDirtyLabel(page string) string {
	def, ok := nativeSettingsDefinition(page)
	if !ok {
		return ""
	}
	if page == settingsPageActionsOnSave {
		return def.page.title
	}
	return def.page.title
}

func nativeSettingsSavingError(page, key string, err error) string {
	return fmt.Sprintf("Could not save %s %s: %v", page, key, err)
}
