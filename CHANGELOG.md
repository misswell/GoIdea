# Changelog

The GitHub release notes, which **Help › What's New** shows, come from the
`## <version>` section here. Every release needs one, or its build fails.

## Unreleased

- The project title-bar menu now lists actions, open projects and recent projects with filtering.
- The native macOS title row is gone, leaving OpenIDEA's integrated window toolbar.

## 0.1.40

- **Run** now uses OpenIDEA's compact session tabs, close controls and vertical action rail.
- **Run** consoles color normal, error and system output and support find, follow and soft-wrap.
- **Run** test sessions show the native results split, filters and selected-test output.
- **Run** empty windows show OpenIDEA's three ways to start a configuration.

## 0.1.39

- Settings import/export and JetBrains IDE imports now use real backend RPCs.
- Build tool actions honor module targets, reload arguments and Maven/Gradle cancellation.
- Run/Debug compatibility actions preserve explicit configuration and process targets.
- Debug consoles keep final program output that arrives after the process terminates.
- **Settings for New Projects** now edits only global defaults on every page.

## 0.1.38

- Structural Search now keeps its channel results when the backend reply arrives first.
- This release includes 0.1.35's native workbench expansion and 0.1.36's bookmark crash fix.

## 0.1.37

- This release includes 0.1.35's native workbench expansion and 0.1.36's bookmark crash fix.

## 0.1.36

- Bookmark shortcuts no longer crash when no document backs the active editor.

## 0.1.35

- **Plugins** can import IntelliJ theme files and installed JetBrains IDE themes.
- Plugin Services logs now find, clear, soft-wrap and toggle options from native actions.
- Plugin content actions route to native editors or report when no editor is active.
- **Auto Import** can add imports after paste automatically or ask which imports to add.
- Terminal mouse modes now match xterm tracking rules for resets, releases and alternate screens.
- **Debug › Rerun** waits for the active session to stop before restarting it.
- **Code Completion** shows parameter information after typing a call delimiter.
- **Git** merge, rebase, push, fetch and update now cover every project repository.
- **Commit** can commit changes across every project repository in one action.
- **Commit** can include individual hunks while leaving other changes unstaged.
- Rollback safely handles staged renames, additions, folders and nested repositories.
- **Undo Commit** blocks unsafe history rewrites and explains the reason.
- Deleting a branch held by a linked worktree offers safe cleanup and explicit force removal.
- **Commit** remembers 20 recent messages and can reuse them from a native history menu.
- **Amend** loads the previous message and restores the untouched draft when unchecked.
- **Commit** pre-fills Git merge/cherry-pick/revert messages and configured commit templates.
- The **Changes** context menu can move changed files to the Trash after confirmation.
- **Git Log** filters commits by author, message, branches, paths and dates.
- Rejected pushes offer **Merge**, **Rebase**, remembered auto-update and bounded retry.
- Git file creation and deletion confirmations support Ask, silent update and never modes.
- **Settings › Version Control › Git** can show inline blame for the current line.
- Gradle tests show structured results and can rerun only the failed tests.
- Published Gradle catalogs provide library, version and bundle navigation and completion.
- Generated Gradle catalog TOML files now navigate `version.ref` and bundle members.
- Editing a Gradle catalog or settings file refreshes catalog navigation immediately.
- **File › Workspace** switches to the created workspace after a completed run.
- Generate Maven Sources applies to a selected POM or every non-aggregated project.
- POM completion replaces the complete existing coordinate or version value.
- Build actions honor module, artifact, before-launch and CMake targets.
- **Before launch** build tasks wait for success, and failure or **Stop Build** cancels the launch.
- Failed hidden builds open the Build window while successful hidden builds stay hidden.
- **Settings › Build Tools › CMake** edits every native profile and its build options.
- **Settings › Editor › Code Style** exposes indentation, wrapping, marks and language overrides.
- **Settings › Editor › Color Scheme** adds scheme, semantic highlight and rainbow bracket controls.
- **Color Scheme** can import, export and delete `.icls` files with native file dialogs.
- **Semantic Highlighting** tints language-server locals; **Rainbow Brackets** colors nesting depth.
- Formatting honors **Do not format**, **@formatter:off** and **@formatter:on** markers.
- **Plugins** logs support native find, soft-wrap, error filtering and clear actions.
- Common IntelliJ macOS shortcuts now reach declaration, usage, completion and refactoring actions.
- **Alt+Shift+Enter**, **Cmd+P** and **Ctrl+Shift+P** open Quick Fix, Parameter Info and Type Info.
- **Generate…**, **Implement Methods…** and **Override Methods…** show native choosers.
- **Refactor This…** orders refactorings and adds **Rename…** and **Safe Delete…**.
- **Cmd+Shift+F7** highlights usages, with **Ctrl+Alt+Up/Down** to navigate them.
- **Cmd+8** opens or closes the **Services** tool window.
- **GitHub Pull Requests** opens in the left tool window with current-branch **Review Mode**.
- Pull request diffs switch between review and normal views with line and multiline comments.
- Review comments can be navigated and submitted as Approve, Request Changes or Comment.
- Review threads can be resolved or reopened from the diff or thread list.
- Pull request title, description and base branch are editable, and timeline comments can be added.
- **GitHub Pull Requests** 可通过原生表单创建 PR、更新分支并在浏览器打开。
- **.NET** tool window discovers projects and solutions and exposes build, restore and clean.
- The **.NET** tool window lists NuGet packages, searches versions and changes dependencies.
- **.NET** tests can be discovered, run individually and shown from TRX reports.
- 关闭项目、退出或切换远程环境前，若仍有运行任务会先请求确认。
- **Run** 设置现在按项目加载、保存，并支持项目级“不要再次询问”。
- **File › Export Settings…** and **Import Settings…** save settings and templates.
- **File › Import Settings from JetBrains IDE…** previews supported editor options.
- **File › Rename Project…** and **Settings for New Projects…** use native flows.
- **File › Power Save Mode** pauses completion, documentation, usage highlights and inline hints.
- **Find** fields grow across lines, retain recent searches and can scroll to results while typing.
- **Window** can move tabs between native editor groups, merge groups and cycle their focus.
- **File › Open Project in New Window** opens a separate project with its own backend.
- **Window › New Window** creates an empty native window, and window commands cycle projects.
- Projects can open in a new window, the current window or after an **Ask** choice.
- The terminal now renders cursor-addressed full-screen TUI output and restores shell scrollback on exit.
- Terminal keys, mouse reporting, paste, resize and full-screen TUI input now work with the shell.
- Vim mode adds search, substitute, named registers, macros, marks and Visual Block edits.
- Markdown files support **Editor Only**, **Editor and Preview**, and **Preview Only** layouts.
- HTML files with Django template tags show a banner that can set the Django Template file type.
- Image files open in a native viewer with zoom, actual size, fit and chessboard controls.
- Binary and very large files open in a virtualized, read-only native hex viewer.
- **Plugins** can install a local folder, update Git plugins and reload all plugins.
- Plugin commands now use OpenIDEA's `plugin.<pluginId>.<commandId>` action ids.
- **Plugins** can color and describe text in editors.
- Plugin gutter dots open their target or a target chooser for multiple matches.
- Indexed plugin markers follow edits, refresh on index changes and request up to 64 files.
- **File › Project Structure** 新增七分区原生对话框，可编辑模块、依赖、库、Facet、Artifact 和 SDK。
- **Project View** 新增 **External Libraries**，可展开 SDK、依赖和归档内容。
- **Code › Analyze Code** 的 Inspection Results 可筛选、导出并跳转检查问题。
- **Code › Analyze Code** 可查找重复代码，并浏览每个重复片段。
- **Find** 支持结构搜索、模板校验和按匹配项预览替换。
- **Code › Analyze Code** 的 Code Cleanup 可批量应用语言服务器修复并保存。
- **Recent Locations** 会为代码片段显示语法高亮和当前文件的符号路径。
- **File Structure** 可从实时大纲筛选符号并直接打开其声明。
- **Copy Path/Reference** 在光标处显示原生路径与引用选择器。
- **Debug** 可并行运行多个 Java 会话，并分别保留变量、控制台、能力状态与消息路由。
- **Debug** 冷启动会等待 Java 导入并重试暂态取消，不再偶发停在 `cancelled`。
- **Debug › Memory** 可浏览 JVM 实例、字段和引用者，并在恢复运行后丢弃旧回复。
- **Debug › Heap** 可管理多个快照、查看类与支配对象、追踪 GC Root 路径并比较快照。
- **Debug › Evaluate Expression…** 会在独立原生对话框中按选中帧求值，并可把表达式加入 Watches。
- **Debug** 的 Dart/Flutter 会话新增独立 Hot Reload 与 Hot Restart，不再误走 Java HotSwap。
- **Debug › View Breakpoints…** 可为行、方法和字段断点编辑启用状态、条件、命中次数与日志消息。
- **Debug › Reset Frame** 可从选中帧重置，并在后端或栈帧不支持时显示真实原因。
- **Debug › Reload Changed Classes** 可按会话编译、取消和重载，外部构建完成可直接应用。
- **Debug › Attach** 支持 Java、Node.js、Python、Go、Native 进程和远端调试配置。
- **Debug** 会自动打开被调试 JVM 的附加监听，让暂停中的新 JDK 可直接捕获 Heap。
- Maven 与 Gradle 运行配置可通过真实 JDWP 调试委托启动，并可取消等待中的调试。
- **Settings › Editor › File Types** 可覆盖单个文件类型，并维护忽略名称与排除模式。
- **Project View** 现在按忽略名称隐藏文件，并可按排除模式隐藏或变暗文件与目录。
- **Project View** 与编辑器标签菜单可直接覆盖或恢复单文件类型，新打开的文件立即生效。
- **File › New Scratch File…** 使用 OpenIDEA 后端语言和模板，并把临时文件列在 **Scratches** 下。
- **Project View** 的 **New** 弹层新增 **Scratch File**，可快速选择临时文件语言。
- 从 Find、TODO 等结果跳转到符号链接路径时，光标会落在目标行列。
- **Settings › Editor › Inspections** 可复制、重命名和删除 IDE 或项目检查配置。
- **Settings › Editor › Spelling** 可维护项目与应用接受词以及自定义词典。
- **Settings › Editor › General** 新增 Auto Import、Code Completion、Gutter Icons、Inline Completion、Smart Keys 与 Sticky Lines 原生页。
- **Settings › Editor** 新增 Rich-Text Copy 与 Code Editing 原生设置页。
- Code Completion, inline completion, Markdown and terminal settings now take effect.
- **Settings › Editor › General** applies gutter visibility changes immediately.
- Gutter switches hide run actions, bookmark dots and color previews by group.
- Code Completion honors single-result, special-key, parentheses and docs settings.
- **Smart Keys** honors pairs, selections, indentation, doc stubs and unindent options.
- **Smart Keys** paste honors block or line reformatting and string escaping.
- **Gutter Icons** filters run, bookmark, color and inheritance marker groups.
- **Auto Import** adds one unambiguous fix and optimizes changed imports after typing pauses.
- **File and Code Templates** resets affect only the active default or project scope.
- **Tools › Actions on Save** runs Optimize Imports and Code Cleanup before formatting.
- **Agents › IDE MCP Server** 可立即启停 HTTP 与 Stdio 服务并生成客户端配置。
- MCP 工具执行前显示原生确认框，可允许、拒绝或全部拒绝。
- MCP 工具可打开文件、启动 Agent，并同步编辑器、选区与 Problems 状态。
- **Settings › Editor › File Encodings** sets file, properties and UTF-8 BOM policies.
- **Settings › Editor › File Encodings** can toggle transparent native-to-ascii conversion.
- Standalone files use **Settings › Editor › File Encodings** and keep their encoding when saved.
- Encoded project files with a configured fallback open in the editor instead of the hex viewer.
- New UTF-8 files follow the **Create UTF-8 files** BOM policy.
- **Settings › Appearance & Behavior / Tools** 新增 Language and Region、HTTP Proxy、Terminal 与 Actions on Save 原生设置。
- **Settings › Languages & Frameworks** 新增 Python、Go、Node.js、Markdown、PHP、Ruby、Rust 与 TypeScript 原生设置页。
- **Settings › Languages & Frameworks** 会检测已安装的语言 SDK，并用原生选择器和浏览按钮填写工具链路径。
- **Settings › Languages & Frameworks** 新增 Gradle 与 Maven 页面，可设置运行工具、JVM、用户目录、配置文件和构建参数。
- **Settings › Languages & Frameworks › Python** 会列出项目虚拟环境与自定义解释器，并可创建 venv、uv、Poetry 或 Conda 环境。
- **Python › Dependencies** lists, installs, upgrades, removes and freezes packages.
- **All Interpreters** includes interpreters from trusted recent projects.
- The status bar can switch the Python interpreter and open interpreter settings.
- The status bar can switch a PHP file's language level or follow the linked project.
- **PHP** settings show effective interpreter, Xdebug and language-level status.
- **Gradle** settings use the selected wrapper or installation for real project sync.
- 更新语言 SDK、Java、Maven 或 Gradle 设置后，会立即刷新运行配置与 External Libraries。
- **Settings › Languages & Frameworks** 新增 Prettier、oxfmt 和 Ruff 原生设置页。
- 外部格式化器支持路径、语言、优先提供程序和保存时运行配置。
- **Settings › Deployment** 与 **Run Targets** 可直达现有原生远端配置编辑器。
- **Settings › Tools › SSH Configurations** 可直达现有原生 SSH 连接编辑器。
- **Settings › Editor › General** 可把粘贴或拖入的图片保存到 Markdown 项目的图片目录。
- **Settings › Editor › Breadcrumbs** 可按 MyGo 支持的全部语言启用或隐藏面包屑。
- **Settings › Editor › Code Folding** 可在打开文件时自动折叠首个 import 区。
- 反编译类文件可 **Download Sources** 下载 Maven 源码，并跳转到源码位置。
- 反编译类文件可通过 **Choose Sources…** 选择本地 jar 或 zip 源码包。
- 源码下载栏支持取消、错误提示、迟到回复保护与 **Sources not found** 状态。
- 左右标签栏现在可直接拖动排序，拖到编辑器边缘可建立新分组。
- 预览标签会随会话恢复，不再在重启后变成普通标签。
- **Settings › System Settings** 可迁移并切换本地数据目录，应用后自动重启。
- **Tools › GraphQL** 原生工具窗口可发现 schema、运行查询并查看结果。
- GraphQL 端点与变量支持环境变量、取消请求和失败重试。
- GraphQL 错误提示提供 **Retry** 与 **Show error**，未发送查询文字显示为柔和背景。
- GraphQL 可从端点生成可撤销的 schema，并在编辑器行内运行 introspection。
- GraphQL 配置和未保存的 schema 变更会防抖重新校验。
- **HTTP Client** 支持环境变量请求、响应与历史、取消、Run All 和 cURL 导出。
- **Maven** 工具窗口新增依赖分析、跳转、添加和排除操作。
- **Gradle** 工具窗口新增项目、任务、运行配置和依赖分析。
- **Database** 查询结果可导出为 CSV、TSV、JSON、SQL、Markdown 或 One-Row。
- 导出对话框提供预览、复制、文件写入、列头、行号和转置选项。
- **Database** 数据页可内联编辑、添加和删除行，并通过一个事务提交到数据库。
- 可编辑单元格提供多行与 JSON 编辑器，以及 Boolean、NULL 和 DEFAULT 操作。
- 未提交的表格修改会在翻页、过滤、排序或关闭前阻止丢失。
- **Services** 新增 Docker 连接、容器、Compose、镜像、卷和网络的原生操作面板。
- **Services** 的 Services、Database 与 HTTP Client 切换时保留各自状态。
- **Deployment** 可在本地文件系统目标上上传、比较、下载和远程删除文件。
- 远端 WSL、Dev Container、端口转发与插件文件读取使用统一原生流程。
- Docker 操作支持日志跟随、事件刷新、inspect、stats、终端、pull 和 registry login。
- **Diff/Compare** 使用可编辑原生双栏，支持同步滚动、变更高亮和接受/拒绝。
- 项目树、编辑器和标签页可直接比较文件、目录或剪贴板内容。
- **Find** and **Replace** work in editable Diff panes, including Undo.
- Read-only revision panes support **Find**, and diffs have overview markers.
- Merge overview markers update as sides are accepted, ignored or edited.
- **Run/Debug Configurations** 新增类型模板、分组新建菜单，并保留检测与临时配置状态。
- **Run/Debug Configurations** 新增类型与文件夹树，可折叠、新建、重命名、删除和拖动配置。
- **Run/Debug Configurations** 可编辑 Maven/Gradle 任务、包装器、VM 参数与激活 profile。
- **Run/Debug Configurations** 可编辑 Before launch 顺序和 Compound 成员，并校验缺失目标与循环。
- **Run/Debug Configurations** 的 Run、Debug、OK、Apply 与 Cancel 可启动、保存、继续编辑或放弃。
- **Run** 选择器可在运行与调试模式使用 Current File，并打开临时配置进行保存。
- **Run** 选择器新增 Pinned、Recent、数字快捷键、F4 编辑、Delete 删除和活动会话操作。
- **Run** 的 Pinned、Recent、临时配置和当前选择会按项目保存并在重启后恢复。
- **Run** 最多保留五个临时配置，并按当前工具链刷新过期的派生字段。
- **Run** 删除临时配置或源文件后会清除失效选择，不会误启动其他配置。
- **Run** 未知测试工具也会用全部内置格式解析并汇总到结果树。
- **Run** 普通运行配置不会再收到只用于测试结果的启动参数。
- **Run** 插件测试配置通过 OpenIDEA 后端流式解析，并可按失败项重跑。
- **Run** shows configurable numbers of finished and archived agent tabs.
- **Run** keeps live agent tabs visible and removes the oldest archived tabs first.
- **Run › Update Application** 可更新 Spring Boot 与 Tomcat，并重启全部活动运行、服务和调试会话。
- **Run › Update Application** 可按运行配置在窗口失焦时自动更新资源或类。
- **Editor** 可导航 Gradle 版本目录的版本、库、插件和 bundle 引用。
- 状态栏可显示 GoIDEA 堆内存、GC 和 `openidea-server` 进程指标。
- **Help › Clear Caches** 可释放未使用的 Go 内存并刷新性能数据。
- **Help › Toggle Performance Overlay** 可查看原生渲染和后端连接指标。

## 0.1.34

- **Run** 可显示 PHPUnit/Pest、Minitest、Django、unittest 和 .NET 测试树。
- **Rerun Failed Tests** 可为这些框架生成过滤条件并只重跑失败项。

## 0.1.33

- **Git › Manage Remotes…** 可查看、添加、编辑和删除仓库远端。
- **Git › Worktrees…** 可创建、打开、移动和删除工作树，并保护当前项目。

## 0.1.32

- 快捷键录制现在正确保留 Ctrl 与 Command 的独立含义。

## 0.1.31

- **File › New…** 提供可搜索的文件模板，并按语言生成包名和文件内容。
- **Project View** 的 **New File** 现在打开 OpenIDEA 同款锚定搜索弹层，并列出文件、目录和模板。
- **Project View** 的移动、重命名和复制使用原生对话框，引用修改会在 **Refactoring Preview** 中预览。
- **View › Appearance** 新增 **Compact Mode** 和 **Navigation Bar** 的三种位置。
- **Navigate › Jump to Navigation Bar** 可用键盘打开项目、目录、文件和符号路径。
- 状态栏右键菜单和 **View › Appearance** 可切换每个已命名 Widget。
- **View › Background Tasks** 无任务时也可打开，并可停止全部后台进程。
- 工具窗口内按 `Cmd+W` 优先关闭当前内容标签页，关闭后隐藏空窗口。
- **Window › Tool Window Layouts** 可保存、恢复、重命名和删除命名的工具窗口布局，`Shift+F12` 恢复默认布局。
- **File › Open File…** 可在无项目时用 LightEdit 打开文件并保存。
- **Tools** 菜单可安装或卸载 `goidea` 终端命令，并用它打开文件或项目。
- **File › Workspace** 可创建、管理和打开多项目工作区，并附加或解除项目。
- 窗口标题和项目菜单显示 OpenIDEA 工作区的组合名称。
- **File › Remote Development** 提供原生远程连接入口，并明确显示当前后端限制。
- **Reload Environment** 刷新新终端和新进程使用的登录 shell 环境变量。
- **View › Tool Windows › Remote Development** 显示远程主机状态和端口转发状态。
- **Code › Show Usages** 新增原生浮窗，可搜索、预览并转存到 **Find Usages**。
- **Refactor › Safe Delete…** 可检查 JDT 引用、预览注释和字符串，并删除声明。
- **Settings › Languages & Frameworks › Language Servers** 可搜索、安装、重装、卸载并配置工具。
- 语言服务器可按项目或全局保存命令、参数、环境变量、初始化选项和语言设置。
- 自定义语言服务器可从 **New Language Server** 添加，并可恢复默认配置。
- 删除期间发生编辑时，**Safe Delete** 会取消并保留原声明。
- 启动预检会校验解压后的 `openidea-server`，不再把正确下载误报为已替换。
- **Branches** 弹窗支持创建、更新、推送、合并、变基、比较和重命名分支。
- 删除本地分支后可恢复并查看未合并提交，删除远程分支前会确认。
- **Local History** 可比较历史版本与当前文件，也可比较同一文件的两个历史版本。
- **Switcher** 与 OpenIDEA 一致地省略搜索框，并保留 `Ctrl+Tab` 的文件与工具窗口切换。
- **Reopen Closed Tab** 会恢复关闭时的选区、滚动位置和撤销历史。
- **Switcher** 可从侧边打开 **What's New**、Agent 会话和 Agent 新任务页面。
- **Settings › System Settings › Date Formats** 支持覆盖日期格式、12/24 小时制和相对日期。
- Git Log、Local History、Code Vision 和 Git Blame 现在统一遵循 **Date Formats**。
- **Annotate with Git Blame** 在编辑器中显示作者与日期栏，悬停可查看完整提交、邮箱和消息。
- **Settings › Version Control › Issue Navigation** 可将提交消息里的问题编号链接到 JIRA、YouTrack 或 GitHub。
- 修复 Java 导入结束附近停止 **Debug** 后会话状态残留的问题。
- jdtls 完成导入后，后台索引或构建不再把 Java 就绪状态短暂改回导入中。
- 退出 GoIDEA 时会等待工作区会话写入完成，避免最近状态丢失。
- 退出 GoIDEA 时会取消工具下载并清理其进程组，不再遗留 `curl`。

## 0.1.30

- 底部工具窗口横跨工作区，调试期间可同时查看左右侧栏。
- **Debug** 启动时显示配置标签和完整宽度的 **Console**，断点命中后显示 **Threads & Variables**。
- **Debug › Memory** 可读取和筛选 JVM 类，**Console** 可搜索输出、换行和求值。
- **Git** 菜单、项目树和标签页新增分支、标签、抓取、拉取、推送、合并、变基、暂存与重置。
- **Git › VCS Operations…** (`Ctrl+V`) 提供可筛选的原生 Git 操作快捷列表。
- 修复 Java 项目冷启动导入期间 **Debug** 将 `busy` 误报为失败，现会等待 JDT 完成后继续。
- 项目树超出窗口宽度时可横向滚动，长文件名保留完整内容，不再用省略号截断。
- 修复更新提示气泡关闭按钮，并在重启后保留通知记录和已读状态。
- **Run › Run Context Configuration** 可按当前文件或测试生成运行配置。
- Maven 和 Gradle 工具窗口补上刷新、取消、展开折叠、离线和跳过测试等操作。

## 0.1.29

- **Run › Run with Coverage** 和 **Coverage** 工具窗口支持运行、导入和查看覆盖率。
- **Code › Quick Documentation** 支持从编辑器查看文档，也可停靠在 Documentation 窗口。
- **Navigate** 的类、文件、符号和操作快捷键进入 **Search Everywhere** 对应页签。

## 0.1.28

- Java 正在导入或索引时，**Debug** 由后端准备启动，支持用 **Stop** 取消。

## 0.1.27

- 在 **Navigate › Go to Line:Column…** 或状态栏输入行列号，直接跳转到对应位置。
- **Navigate** 新增 **Recent Files** 和 **Recent Locations**，支持按文件名、代码和编辑记录筛选。
- **Code › Analyze Stack Trace or Thread Dump…** 在 **Run** 中打开堆栈，点击帧可跳转到源码。
- **Build › Compile File** 可以单独编译 Maven 项目中的 Java 文件。
- 恢复 Maven 工具窗口的构建图标，包括离线模式和依赖分析。

## 0.1.26

- 界面换用与 OpenIDEA 相同的 Inter、JetBrains Mono 字体，字号与行高一致。
- 项目树、更改、结构、书签、TODO、问题、调用层级、查找用法与 Git Log 统一为 OpenIDEA 的行高、间距与选中样式。
- 更改列表按 OpenIDEA 用颜色区分新增、修改、删除和未跟踪文件，提交对话框同款文件图标。
- 问题窗口使用严重性图标，数据库、调试与终端窗口补齐 OpenIDEA 的图标和文字大小。
- 更新通知气泡不再显示 Markdown 符号，发布说明按纯文本展示。

## 0.1.25

- **Check for Updates** 按 OpenIDEA 改为后台任务：检查与下载显示在状态栏，可从 **Background Tasks** 弹窗取消。
- 检查结果显示为通知气泡：有新版本时提供 **Install and Restart**，已是最新或检查失败时给出说明。
- 下载显示 `x MB of y MB` 进度；校验发布包的 `.sha256` 后替换应用并重启，取消或失败可 **Retry**。
- 启动后自动检查一次，之后每 4 小时检查一次；每个新版本只提示一次。
- 新增 **Settings › Updates**：当前版本、上次检查时间与 **Check Now**、自动检查开关、更新后显示 What's New。
- 新增 **Help › What's New**：以页面展示当前版本的发布说明，可勾选更新后是否再次显示。
- 状态栏显示正在进行的任务及其进度，点击可查看任务列表。

## 0.1.24

- 启动先显示欢迎页，不再自动打开上次的项目；可在欢迎页 **Customize** 勾选恢复。
- 新增 **File › Close Project**：关闭项目并回到欢迎页，应用继续运行。
- **Help › Check for Updates…** 支持后台检查、更新说明、带校验的下载安装与自动重启。

## 0.1.23

- Java 项目导入完成前点 **Debug** 不再报错：等待导入结束后自动继续，期间可 **Stop** 取消。

## 0.1.22

- 修复从 **Maven** 窗口运行生命周期阶段失败：现在发送完整的运行配置（含 `isTest`、`debugAdapter`），不再被后端以 `missing field isTest` 拒绝。

## 0.1.21

- 编辑器标签页按文件类型显示图标（Java/XML/YAML/JSON/脚本等），与项目树使用同一套图标。

## 0.1.20

- 状态栏补齐 OpenIDEA 的项：语言、`Ln 1, Col 1 (4 chars)`、`4 spaces`、`LF`/`CRLF` 与 `UTF-8`。

## 0.1.19

- 左侧项目树按 OpenIDEA 呈现：包链合并为 `com.example` 一行，模块/源根/包与按类型区分的文件图标，`target`、`.openidea` 等排除目录以琥珀色底显示。

## 0.1.18

- **Maven** 工具窗口按 OpenIDEA 重做：工具栏、Build files changed / **Load Maven Changes** 横幅、Profiles、Lifecycle/Plugins/Dependencies/Run Configurations 与模块层级。
- 双击生命周期阶段、插件目标或 Maven 运行配置即可运行；**Work Offline** 与 **Skip Tests** 开关会写进命令行。

## 0.1.17

- **Add SQLite…** 添加的数据源现在能真正连接：数据源 id 不再含冒号，与后端 `<id>[:<database>]` 的目标格式一致。
- 数据库的**表数据**、**执行历史**与**执行计划**子窗口显示在数据库窗口之上；历史与计划列表按行数收缩。

## 0.1.16

- 界面语言统一为英文：菜单、命令面板、设置、对话框、提示条与状态栏不再中英混排。
- 计数文案按单复数显示（`1 item` / `2 items`），运行配置表单的英文标签不再被截断。

## 0.1.15

- **Run › Edit Configurations…** 现在会打开配置对话框：此前对话框以关闭状态创建，命令没有反应。
- 差异视图的增删行与区块标题改为淡色底，之前整行是实色块、文字几乎看不清。
- 后端返回的错误信息中的 OpenIDEA 统一显示为 **GoIDEA**，保留 `openidea-server`、`.openidea` 等真实名称。

## 0.1.14

- **File › New Project…** 现在会打开向导：此前命令和欢迎页按钮都不显示任何界面。
- 无干扰模式（**View | Appearance**）恢复显示编辑器内容与退出提示条，不再是一片空白。
- 通知气泡的高度计入「Don't Show Again」，底部操作不再被裁掉；未读数徽标收进按钮内。
- **Quick Open** 的结果列表按行数收缩，只有一条结果时不再留下大块空白。

## 0.1.13

- 界面与错误提示统一为 **GoIDEA** 命名：客户端错误前缀改为 `goidea:`，缺失后端时提示「未找到 GoIDEA 后端程序」。

## 0.1.12

- 修复设置、数据库与插件页面的两栏错位：内容从顶部对齐，设置页的搜索框带 `Search settings` 占位符。
- 界面中的产品名统一为 **GoIDEA**，状态栏显示「GoIDEA 后端已连接 / 未连接」。

## 0.1.11

- 修复安装版启动失败：签名后的内置 Server 与校验和文件不一致，导致预检拒绝启动；发布流程改为按出厂字节重写校验和并重新封装。

## 0.1.10

- 测试结果管线支持 **jest**：verbose 输出解析进结果树，重新运行失败的测试按 `-t` 正则锚定派生（项目需安装 jest）。
- 结果树中双击失败的测试会打开其源文件并定位到失败行。

## 0.1.9

- 测试结果管线支持 **pytest**（`-v` 详细行、摘要与失败段落）：输出解析进结果树，重新运行失败的测试按 node id 派生。

## 0.1.8

- 测试结果管线支持 **cargo test**（libtest 与 nextest）：输出解析进结果树，重新运行失败的测试按 `--exact` 过滤派生。
- 新增**开箱即用全链路回归**：全新数据目录从工具链安装到六态就绪再到真实 Maven 运行（发布门禁）。

## 0.1.7

- 编辑器右键菜单新增 **Local History**：显示历史时间线与**添加标签…**，标签行可一键把目录内所有文件恢复到标签时刻。

## 0.1.6

- 测试结果管线支持 **go test**（含 `-json`）：运行 `go test` 输出解析进结果树，重新运行失败的测试按 `-run` 锚定派生。

## 0.1.5

- 运行窗口新增**测试结果树**：状态标记、耗时与失败详情，点击查看单个测试的输出。
- 新增 **附加到进程…**（`Alt+Shift+F5`）：枚举可附加的 Java（JDWP）进程并附加到运行中的 JVM，挂起启动的 VM 附加后自动恢复。
- 新增启动就绪状态机：后端连接 → 项目打开 → 导入中 → Java 就绪 → 构建模型就绪 → 运行就绪，未就绪时显示原因。

## 0.1.4

- 运行窗口新增**重跑失败测试**：按原版解析 surefire 输出生成结果树，重跑只带失败的方法（`-Dtest=类#方法`）。
- 新增 **重新加载变更的类（HotSwap）**：jdtls 编译后把变更类重定义进运行中的 java-debug 会话，jdtls 自动构建完成会提示重载。

## 0.1.3

- 新增 **选择运行/调试配置…**（`Ctrl+Alt+R`）弹出配置选择器：Enter 运行、`Shift+Enter` 调试、`Ctrl+Alt+D` 直接进入调试模式。
- **调试**（`F9`）现在调试所选的运行配置，**调试当前文件**移至 `Ctrl+Shift+D`；两者共用同一套启动准备。
- 修复项目切换时误报“后端重启后恢复项目失败”的缺陷：首次启动不再运行重启恢复回调。
- 新增运行准备负向回归：缺少 JDTLS、运行中切换项目、Server 在准备期间被杀——全部验证拒绝启动且无孤儿进程。

## 0.1.2

- 新增 **编辑运行配置…**（`Ctrl+Alt+Shift+R`）：编辑名称、命令、实参、工作目录、环境变量、模块与启动前构建，保存即写入 `.openidea/run.toml`。
- 运行中的配置可以**重新运行**（`Cmd+R`）：按 OpenIDEA 原版先确认“进程正在运行”，停止并等待退出后再启动，“不再询问”会被记住。
- 启动前构建（Build Before Launch）对齐原版：Maven/Gradle 的 Rebuild 预置 `clean`，Maven 的“构建、不检查错误”附加 failOnError 开关。
- 新增 **PARITY_AUDIT.md** 功能继承审计：全部未集成项按目标五大领域分组并标注复用后端 / 移植流程 / 原生界面 / 上游受阻。

## 0.1.1

- 真实多模块 Maven/Spring Boot 项目已通过内置引擎完成构建，并可从 GoIDEA 启动与停止。
- 后端进程意外退出会自动重启，并恢复原项目和已打开的文件。
- 新增 **Maven** 工具窗口：列出反应堆模块与可展开的依赖树，数据来自后端构建模型。
- 在 `pom.xml` 中对 `${属性}`、依赖、插件或 `<parent>` 跳转声明，父 POM 可从本地仓库打开。
- `Cmd+Alt+B` 转到实现：结果边查找边显示，多个候选弹出选择列表，关闭即取消查找。
- `Cmd+Shift+B` 转到类型声明：打开光标处符号类型的声明文件。
- 切换项目失败时原项目的标签页保持可编辑，不再丢失未保存内容。
- 后端重启后项目恢复失败会报错并标为失败，不再一直停在“正在恢复”。
- **Maven** 窗口刷新后不再卡在“读取中”，切换项目会重新加载构建模型。
- 从菜单打开 **Services** 也会读取构建模型；数据库过期的表结构响应不再清空当前表列表。
- **数据库** 双击表打开表数据页：可翻页浏览行，并可切换查看该表的 DDL。
- 表数据页新增**详情**：列（类型/主键/非空/默认）、索引、外键与触发器。
- **数据库** 控制台新增**历史**（选择并回填已执行语句）和**解释**（查看执行计划树）。
- 调试器新增**运行到光标**（`Alt+F9`）和**强制运行到光标**（`Cmd+Alt+F9`）。
- 终端标签栏新增 **⌄**：从已安装的 Shell 列表中选择并新建终端标签页。
- **Services** 的 Maven/Gradle 构建行改用同一构建模型（模块数量），离线读取失败时才回退到文件检测。
- **Database** 每次运行生成结果标签页，**Output** 保留运行日志，可关闭、关闭其他、切换上下一个结果。
- Java/Spring Boot 运行配置会自动解析模块类路径与 JDK，不再把 `${classpath}` 原样交给进程。
- 项目切换校验后端当前项目，失败时回到原项目，不会留下半打开状态。
- Java 语言服务与调试器已在 v0.1.44 引擎上跑通：补全、悬停、跳转、重命名、诊断与单步调试。
- 上游同步到 **OpenIDEA v0.1.44**：源码、Server 与 RPC 清单一起升级并重新生成 IPC/Parity 清单。
- 启动前检查 Server 是否存在、可执行、架构匹配、校验和与版本/协议，不满足时给出明确原因。
- 启动状态机显示检查、启动、打开项目与恢复进度；项目打不开时留在原项目并报错。
- **Settings**（`Cmd+,`）改为 OpenIDEA 的设置窗：左侧页面树和搜索，右侧面包屑和页面，可后退和前进。
- 设置窗底部是 **Cancel** / **Apply** / **OK**，改动会先预览，**Apply** 和 **OK** 才写入，**Cancel** 恢复原样。
- 设置页按 OpenIDEA 分组：**Appearance**、**Keymap**、**Editor > Appearance / Font**、**Languages & Frameworks > Java**。
- **Editor > Appearance** 的 **Show line numbers**、**Show whitespaces** 和 **Show indent guides** 会立即作用到所有打开的编辑器并保存。
- **Editor > Appearance** 新增 **Use block caret**，光标可选方块形并随设置保存。
- 新增 **Editor > Breadcrumbs** 设置页，**Show breadcrumbs** 和 **Top/Bottom** 控制编辑器的面包屑并保存。
- 新增 **Editor > General** 设置页，**Soft-wrap files** 和虚拟空间开关直接作用于编辑器并保存。
- **Editor > General** 新增 **Save Files**，保存时删除行尾空格，可选 **Modified lines** 或 **All lines**。
- **Save Files** 还能在保存时删除文件末尾的空行，并保证文件以换行结尾。
- 新增 **Editor > Editor Tabs** 设置页，标签栏可放在顶部、底部或隐藏。
- 标签页可显示扩展名、所在目录、修改标记和完整路径提示，还能按字母排序、新标签排在末尾。
- **Tab limit** 限制未固定标签页的数量，最久未用的先关闭，有未保存内容的标签不会关闭。
- **Editor > Editor Tabs** 的 **Show tabs in** 可选单行滚动、单行压缩或多行换行，切换标签页会滚到可见处。
- 标签页可显示文件图标，**Show file icon** 可关掉它。
- **Tab placement** 可选 **Left** 或 **Right**，标签页变成编辑器旁边一列 140–260 宽的竖排标签。
- 当前标签页改用 OpenIDEA 的浅色选中块和一圈描边，不再用底部色条。
- 标签页多到放不下时，标签栏右端出现下箭头，点开 **Show Hidden Tabs** 可跳到任意标签页。
- 拖动标签页可调整顺序：原来位置留出空位，其余标签让开，松手即按新顺序排列。
- 标签页右键菜单新增 **Keep Tab Open**、**Open In** 和 **Local History**。
- 文件标签页的右键菜单还能 **Copy Path/Reference** 和 **Rename File**。
- 标签页右键菜单的 **Configure Editor Tabs…** 直接打开标签页设置页，**Close** 显示 `Cmd+W`。
- 编辑器可拆成两组：**Split Right**（`Cmd+\`）和 **Split Down** 并排或上下显示同一文件的两个视图。
- 标签页右键菜单和 **Window › Editor Tabs** 还能 **Split and Move**、**Move to Opposite Group** 和 **Open in Opposite Group**。
- **Unsplit**、**Unsplit All** 合并两组，**Change Splitter Orientation** 在并排和上下之间切换。
- `Alt+Tab` / `Alt+Shift+Tab` 在两组之间切换，点另一组的标签页或面包屑也会把焦点带过去。
- 两组编辑器随项目保存，重开项目后回到原来的排列和活动标签页。
- 标签页右键菜单新增 **Compare With…**，可把当前内容（含未保存修改）与另一个文件比较。
- 标签页右键菜单新增 **Compare with Clipboard**，把当前内容与剪贴板里的文本比较。
- 项目内文件的标签页右键菜单新增 **Git** 子菜单：**Commit File…** 只提交该文件，新文件也能一起提交。
- **Git** 子菜单还有 **Add**、**Show Diff**、**Show History**、**Rollback…** 和 **Branches…**。
- **Git** 子菜单新增 **Annotate**，按提交分组列出每一行最后是哪次提交、谁、何时改的。
- 编辑器的右键菜单新增 **Show Context Actions**、**Copy / Paste Special**、**Go To**、**Reformat Code** 和 **Rename…**。
- **Copy Path/Reference** 可复制编辑器的绝对路径、文件名、项目内路径和带行号的路径。
- **Editor > Editor Tabs** 可选择关闭当前标签页后激活哪个：左边的、右边的或最近打开的。
- 单击 **Problems**、**Find**、**Find Usages** 的结果在斜体预览标签中打开，下一次单击原位替换它。
- 编辑、固定或双击结果会把预览标签转为普通标签；**Editor > Editor Tabs** 的 **Opening Policy** 可关掉预览标签。
- 命令面板新增 **Reopen Closed Tab**，最多记住 50 个关闭的标签页，重开时回到原来位置并保留固定状态。
- 齿轮菜单新增 **Run Anything…**：搜索最近命令和运行配置，回车在项目目录执行或在运行窗口启动配置。
- 工作台改用 OpenIDEA Islands 布局：顶部标题栏、左侧图标条、独立面板和 34px 状态栏。
- 底部工具窗口现在只占编辑器下方，左侧项目岛保持从标题栏到状态栏的整高。
- 左侧图标条再次点击当前面板会折叠它，再点一次恢复，**Search Everywhere** 固定在底部。
- 没有打开文件时，编辑器居中显示 **Search Everywhere** 和 **Go to File** 提示，可直接点击。
- Welcome 按 OpenIDEA 原页面重做：左侧品牌栏和 Projects、Customize、Plugins 页面，右侧项目搜索、操作和最近项目。
- 最近项目显示渐变首字母图标、路径和 Git 分支，悬停出现菜单，可打开、在访达中显示、复制路径或移除。
- Welcome 的 **Clone Repository** 用 `git clone` 把仓库克隆到所选目录并打开。
- Customize 页面可切换颜色主题、同步系统外观和调整编辑器字号，**All settings…** 打开设置。
- Welcome 的 **Plugins** 页面读取真实插件目录，分 **Available** / **Installed** 两栏，可搜索、查看详情并开关插件。
- **Plugins** 页面可从 **Install Plugin from Git…** 安装插件，详情里可 **Reload** 或 **Remove**，删除前会确认。
- **显示问题** 在底部列出当前文件或整个项目的错误、警告和信息，点击跳到所在行，状态栏显示计数。
- 工具窗口使用 OpenIDEA 的快捷键：**项目** Cmd+1、**查找** Cmd+3、**运行** Cmd+4、**调试** Cmd+5、**问题** Cmd+6、**Git** Cmd+9、**终端** Alt+F12。
- 标签页切换从 Cmd+数字 改为 OpenIDEA 的 Ctrl+→ / Ctrl+←，以及 Cmd+Shift+] / Cmd+Shift+[。
- 标签页可固定，固定的标签停在最左侧并显示图钉，右键标签可固定、关闭其他、左侧、右侧或未修改的标签页。
- 固定的标签会随项目恢复，**关闭所有标签页**时保留，**关闭未固定的标签页**只留下它们。
- **视图** 菜单按 OpenIDEA 再按一次 Cmd+6 隐藏问题窗口，**Shift+Esc** 隐藏当前工具窗口。
- **视图** 菜单新增 **隐藏全部工具窗口**（Cmd+Shift+F12），再按一次恢复刚才的窗口。
- **视图** 菜单新增 **F12** 跳转到上一个工具窗口，**Cmd+0** 打开提交窗口。
- **下一个问题**（Cmd+Alt+↓）和 **上一个问题**（Cmd+Alt+↑）在问题列表中循环跳转。
- 底部主区和右侧副区各显示一个工具窗口，**移动到底部右侧**让两个窗口并排，被换下的窗口内容保留。
- **Navigate › Search Everywhere** 打开搜索窗，All/Classes/Files/Symbols/Actions/Text 页签分段列出结果。
- 右下角弹出通知气泡：构建结果与错误会提示，悬停暂停消失，错误气泡保留到手动关闭。
- 右侧条纹新增 **Notifications** 铃铛和未读计数，窗口里保留历史，**Clear All** 清空。
- 设置新增 **Appearance & Behavior › Notifications**：全局气泡开关、每组 Balloon/Sticky/No popup 和 Log。
- 新增 **Services** 工具窗口（**Window › Tool Windows › Services**），列出构建系统、运行配置与工具设置，双击即执行。
- **Problems** 和 **TODO** 可在右侧与底部之间移动：**Window › Move Tool Window to Right / Bottom**。
- 移到右侧的 **Problems**、**TODO** 不再显示空白；右侧上下可各放一个窗口，中间分隔线可拖动调整比例。
- **Customize Main Toolbar** 加入的操作现在真的出现在标题栏，以操作标题显示，上下文不可用时自动隐藏。
- 打开未信任项目或直接运行时会弹出 **Trust Project**，信任后继续运行。
- 插件的 `views` 出现在侧边工具窗口，带条纹按钮，点击按钮或列表项会回调插件。
- 插件可向状态栏添加条目：文本、提示和点击命令，禁用或移除插件时自动消失。
- **Plugins** 启动时加载已启用的插件，插件命令自动进入命令面板，并可读取项目内文件。
- **Settings › Notifications** 新增 **Play sound**，气泡弹出时播放系统提示音，被静音的分组不响。
- **Search Everywhere** 无结果时显示居中的空、加载或错误状态，与其他面板一致。
- 关闭最后一个弹窗后键盘焦点自动回到编辑器，不再停留在空处。
- 按住工具窗口条纹按钮拖到工作区边缘即可停靠，下半部或右侧会落到副插槽。
- **Window › View Mode** 新增 **Dock Pinned / Dock Unpinned**：未固定窗口在焦点移开时自动隐藏并随项目保存。
- **Window › Pin Tool Window** 固定当前窗口：别的窗口会占用同侧另一半，两侧都固定时提示等待。
- **Search Everywhere** 页签新增 **Filter**：按符号种类筛选，**Text** 页可填文件掩码，设置会保存。
- **Services** 显示运行配置的状态和退出码，运行中的进程多出一行 **Stop** 可直接停止。
- **Search Everywhere** 空查询时先列 **Search History**，最近搜索词可回车重放并随会话保存。
- **Window › Move to** 提供 Left Top / Left Bottom / Right Top / Right Bottom / Bottom Left / Bottom Right，移动不丢内容。
- 状态栏显示当前 **Git** 分支和构建进度，点击分支即打开分支弹窗。
- 工具窗口的位置、隐藏与恢复按六个插槽保存，旧会话打开项目时会自动迁移。
- 全部工具窗口（Run、Debug、Build、Terminal、Git Log、Find、Usages、Structure、Bookmarks）改用同一套插槽与注册表，移动或隐藏不会清空内容。
- **Settings › Keymap** 显示实际生效的键位，标出冲突，可逐项移除或**恢复默认键位**。
- **Welcome** 最近项目可分组：新建、重命名、删除、移入移出，分组可折叠并随会话保存。
- **Welcome** 搜索改为名称与路径的模糊匹配，命中字母高亮，方向键选择、Enter 打开、Esc 清空。
- 最近项目会记住打开时间、分支和所属分组，旧会话首次读取时自动迁移。
- **File › New Project…** 现在列出 OpenIDEA v0.1.44 的 38 个项目模板（Java、Spring Boot、Kotlin、Go、Rust 等），按模板字段创建并打开真实工程。
- **File › New Project…** 改为真实对话框：名称、位置、**Browse…** 与 **Create Git repository**，创建后直接打开项目。
- 空目录会被复用，非空目录会提示冲突；创建失败在对话框内显示错误。
- **Welcome › Customize** 新增 **Accent color**、**Keymap** 入口和 **Startup**（重开上次项目 / 显示 Welcome），即时生效并保存。
- **Navigate > Call Hierarchy**（`Ctrl+Alt+H`）和 **Type Hierarchy**（`Ctrl+H`）在右侧列出调用链，可逐级展开。
- **查找**（`Cmd+3`）和**查找用法**（`Alt+F7`）的结果改在底部工具窗口显示，不再占用侧边栏。
- **结构**窗口（`Cmd+7`）在侧边栏列出当前文件的成员，可过滤、折叠，点击跳到声明。
- **结构**和**书签**窗口改为打开在**项目**窗口下方的下半区，同一时间只显示一个。
- 拖动**项目**窗口与下方窗口之间的分隔线可调整两半的高度，每个项目记住自己的比例。
- **最大化工具窗口**（`Cmd+Shift+'`）让底部窗口铺满工作区，再按一次恢复编辑器。
- **Ctrl+Alt+↑/↓** 按步调整底部窗口高度，打开文件或切换标签时恢复编辑器。
- **提交**（`Cmd+K`）打开提交窗口，选择要提交的文件，并可修正上次提交、签名和运行 Git 钩子。
- 标题栏显示当前分支、领先/落后数量和进行中的合并或变基，点击打开**分支**弹窗切换分支。
- **更改**面板按**已暂存**/**未暂存**分组，每行可**暂存**、**取消暂存**或**回滚**，回滚前会确认。
- **F3** 在当前行添加或移除书签，编辑器边栏显示书签圆点，**Cmd+F3** 打开书签列表，可跳转或删除。
- **Alt+F3** 为书签选择助记符，**Ctrl+Shift+数字** 标记、**Ctrl+数字** 跳转，助记符在项目内唯一。
- **Cmd+2** 在侧边栏打开书签工具窗口，列出全部书签，点击跳转，可逐条删除。
- **代码** 菜单的**切换文件书签**把当前文件加入书签工具窗口，再按一次移除。
- **新建书签列表…** 把书签分到命名列表，工具窗口按列表分组，新书签加入默认列表。
- 书签工具窗口的列表可**重命名**、删除（连同列表里的书签），也可设为默认。
- **Cmd+F3** 书签弹窗按列表分组，文件书签和行书签一起列出，可跳转或删除。
- 在书签弹窗中按 **F2** 编辑行的书签描述，描述显示在书签列表里。
- 书签工具窗口的每行都有**移动**按钮，把书签换到另一个列表，也能同时新建列表。
- 项目视图的每行悬停显示**书签**按钮，文件和文件夹都能加入书签工具窗口。
- 项目视图右键菜单有**新建**、**剪切**、**复制**、**粘贴**、**复制路径/引用**、**跳到源码**、**在文件夹中查找**、**在终端中打开**、**本地历史**、**比较目录**、**重命名** 和 **删除**。
- 项目视图右键菜单新增 **Git** 子菜单：**提交**、**添加**、**添加到 .gitignore**、**注解**、**显示差异**、**显示历史**、**回滚** 和 **分支**。
- **Git › 提交…** 只列出并提交所选文件夹下的改动，**提交文件…** 只提交右键的那个文件。
- **添加到 .gitignore** 可选写入共享的 `.gitignore` 或只影响本地的 `.git/info/exclude`。
- 项目视图的**在文件夹中查找**把**查找**窗口限定在该文件夹，窗口标出当前范围，`Cmd+Shift+F` 回到整个项目。
- 项目视图的**本地历史 › 显示历史…**列出文件的各个保存版本，选**恢复此版本**再**确认恢复**换回旧内容。
- **本地历史 › 标记版本…**给当前状态起名，之后可在任何文件的本地历史里看到这个标签。
- 项目视图的**比较目录…**输入另一个目录，列出两个目录里不同、只在一边和相同的文件。
- 项目视图的**从磁盘重新加载**和 `Cmd+Alt+Y` 只重读选中的文件夹，不重读整棵树。
- 命令面板的**从磁盘重新加载项目树**重读项目视图读过的所有文件夹。
- 比较目录的结果里点一行，打开这两个文件的差异视图。
- **新建**文件和**新建**目录在项目视图里输入名称，创建后文件直接打开。
- 重命名会同步打开的文件和书签；删除先确认，再关闭它的标签页并移除名下的书签。
- **Cmd+=** 展开项目树的所有文件夹，**Cmd+-** 折叠到只剩根节点。
- **Cmd+Alt+Y** 从磁盘重新加载项目树，**Alt+F1** 把当前文件定位到项目视图并选中。
- **终端**工具窗口可以开多个标签，用 `Cmd+T` 或**新建**打开，每个标签是独立的登录 shell。
- 终端标签上的 **×** 结束该标签的 shell；关掉最后一个标签时工具窗口收起。
- 终端可**拆分**成并排或上下的两个窗格，各自带标签和 shell，**合并**把两个窗格收回一个。
- 菜单栏按 OpenIDEA 重排为 **File**、**Edit**、**View**、**Navigate**、**Code**、**Refactor**、**Build**、**Run**、**Git**、**Tools**、**Window**。
- 没有 Git 仓库的项目在标题栏显示 **Version Control** 菜单，可**Clone…** 或打开 **Show Git Log**。
- 终端里按 **Cmd+F** 打开查找栏，匹配的行会标出，**上一个**/**下一个** 在匹配间循环并滚到该行。
- 终端输出会自动跟随末尾，向上滚动后停在你查看的位置，回到末尾又继续跟随。
- 提交窗口先保存打开的文件，再按 OpenIDEA 的规则把选中的改动加入暂存区后提交。
- **显示 Git 日志** 在底部打开提交历史，可查看提交说明、所在分支、签名和改动的文件。
- 每个项目记住自己的工具窗口布局：左栏窗口、底部窗口、拆分和高度在再次打开时恢复，**恢复默认布局**把拆分窗格并回底部。
- 拖动左侧栏右缘的间隙可调整其宽度，拖过最小值一半再松手会收起，每个项目记住自己的宽度。
- 编辑器改用 OpenIDEA 的 IntelliJ 按键：**Cmd+D** 复制行、**Cmd+Backspace** 删除行、**Cmd+/** 注释行。
- **Alt+Shift+↑/↓** 移动当前行，**Cmd+Shift+↑/↓** 移动语句，**Ctrl+Shift+J** 合并行。
- **代码** 菜单列出注释、复制行或选区、删除行、移动行和语句、合并行，与按键走同一套实现。
- 拖动底部窗格上缘的间隙可调整其高度，拖过最小值一半再松手会收起全部底部窗口，每个项目记住自己的高度。
- **构建项目**（`Cmd+F9`）、**构建模块**（`Cmd+Shift+F9`）、**重新构建项目** 和 **构建工件** 通过 OpenIDEA 构建引擎执行。
- 构建输出显示在底部构建窗口，结束时给出错误数、警告数和耗时，**停止构建** 可取消。
- **Settings > Java 语言服务与调试器** installs jdtls and java-debug with progress, cancel and a retry, without leaving GoIDEA.
- The command palette's **安装 Java 语言服务与调试器** runs the same install.
- Opening a project restores only the session files inside that project, including through `/var` → `/private/var` links.
- Clicking a repository's name or row background in source control expands or collapses its changes.
- **Settings > Shortcuts** lists **Report a Problem** under Help instead of Account.
- A `mayoi://file` link opens its file at the line and column it names.
- LocatorJS's **Custom link** reaches Mayoi with `mayoi://file/${projectPath}${filePath}:${line}:${column}`.
- A `mayoi://` link to a missing file says Mayoi could not open it, in the window in front.
- 没有打开项目时，窗口换成 OpenIDEA 风格的欢迎页：左侧栏目、**新建项目**/**打开**按钮和最近项目列表。
- 欢迎页的**最近的项目**显示文件夹路径，可搜索，点一行直接打开该项目。
- 窗口改用 OpenIDEA 的深色配色，工具窗口改为侧边竖向图标栏，标签页用顶部色条标出当前文件。
- 项目视图标题栏有**全部展开**和**全部折叠**按钮，不用再打开右键菜单。
- 标题栏右键或齿轮菜单的 **Customize Main Toolbar** 可以显示或隐藏部件，**Restore Default Toolbar** 还原。
- 标题栏新增 **Run** 配置控件：选择运行配置、**Run**/**Debug**，运行中变为 **Rerun** 和 **Stop**。
- 齿轮菜单按 OpenIDEA 顺序列出 **Run Anything…**、**Settings**、**Plugins**、**Theme**、**Keymap**、**View Mode**、**Customize Main Toolbar** 和 **Check for Updates…**。
- **View Mode** 的 **Enter Distraction Free Mode** 隐藏工具窗口、工具栏和状态栏，把编辑器居中到设定列宽。
- **Exit Distraction Free Mode** 恢复原布局，模式和列宽会被记住，设置里的 **Appearance** 可调整列宽。
- **Customize Main Toolbar** 打开左右两栏的部件列表，可用 **Add…** 添加、**Remove** 移除、**Move Up/Down** 排序或换到另一侧。
- **OK** 保存工具栏改动，**Cancel** 放弃，**Restore Defaults** 还原默认部件。
- **Check for Updates…** 在后台查询 GitHub 最新版本，显示已是最新、有新版本或检查失败，并可打开发布页。
- **Settings** 改为左侧分类、右侧内容的弹窗，包含 **Appearance**、**Keymap** 和 **Java** 页面。
- 新增 **TODO** 工具窗口，按项目扫描 **TODO**/**FIXME**/**HACK**/**XXX** 注释并在文件下分组列出。
- **TODO** 窗口有 **项目**、**当前文件**、**按范围**、**更改** 四个页签、关键字过滤和范围选择，单击预览、双击跳转。
- **Tools** 菜单新增 **Refresh TODO Items** 和 **Show TODO Items in Current File**。
- 新增 **Editor > TODO** 设置页，可开关 **Case sensitive** / **Only in comments**，增删关键字和配色。
- 编辑器用各关键字的颜色给 TODO 注释加上高亮，输入停下后更新，并随设置保存。
- **文件** 菜单新增 **Reload All from Disk**，重新读取所有未修改的标签页，未保存的修改会保留并提示。
- 新增 **New Scratch File…**（`Cmd+Shift+N`）：选择语言后在数据目录创建临时文件并打开，最近用过的语言排在最前。
- **文件** 菜单新增 **Invalidate Caches…**：可选清除本地历史、语言服务工作区和构建工具缓存，重启后重建索引。
- **文件** 菜单新增 **Restart IDE**：只重启应用，有未保存的修改时仍会先询问。

## 0.19.3

- Language server plugins can run a `resolve` command that sets the server's arguments, environment and settings.
- Language server plugins can map paths so definitions and diagnostics keep the workspace's symlinked paths.
- Mayoi no longer waits on a language server that has already stopped.
- The command palette, Quick Open and pickers show at once when a key opens them, and fade in on a click.
- Animations share a few lengths, so what moves alike now moves alike.
- Choosing a tab no longer makes its title bolder and shifts the tabs after it.
- The caret's line and column in the status bar, and find's match count, no longer jiggle as digits change.
- Remote SSH and Mermaid commands show their menu in the command palette instead of a bare `:`.
- A terminal whose shell exits as it starts stays open with what it printed, and offers **Report…**.
- **Help → Report a Problem…** opens a new GitHub issue with Mayoi's version and your system filled in.
- Inline blame, commit tips and settings sync show a date for anything older than a week.
- The merge view's count of resolved files reads "1 of 1 file resolved" in English.
- Inline blame now shows with **View → Word Wrap** on, after the caret line's last row.
- When a wrapped line's last row is too full, its inline blame is cut short with `…`.
- Dragging over code in a diff selects text across lines; dragging on the line numbers still selects rows.
- Double-clicking a word in a diff selects it, and a triple click selects the line; `E` opens the file.
- `Cmd+C` in a diff copies the selected code with its indent, without line numbers or `+`/`-` marks.
- Side by side, a selection stays on the side where it started.
- `Cmd+F` in a diff tab finds in the lines it shows on both sides, in every file, with a match count.
- `Enter` and `Shift+Enter` in a diff's Find go to each match, unfolding a collapsed file to show it.
- **Git → Collapse All Files in Diff** and **Expand All Files in Diff** fold the current diff's files.
- On macOS, `Option+Cmd+[` collapses all diff files and `Option+Cmd+]` expands them.
- `mayoi settings` reads and changes preferences, with Mayoi running or not.
- `mayoi settings --folder` in a project's subfolder changes the project's `.mayoi/settings.json`.
- Choosing **Overwrite** for a remote file that changed on its host no longer asks again while an autosave runs.
- After a remote connection is lost, the banner shows its first try to reconnect as attempt 1.
- A question that comes up in the background now waits instead of replacing the dialog you're answering.
- Questions that come up in the background, as a remote file's conflict, wait for an open picker to close.
- Pressing `Cmd+Q` or a window's close button again while the unsaved-changes question shows brings it forward.
- Saving a remote file while another save is under way no longer loses a pending save and close.
- **Revert** or `Escape` on the "changed on host" question keeps the tab open instead of dropping the save.
- Autosave no longer writes to the host while the "changed on host" question waits for an answer.
- With format on save, saving twice in a row waits for the formatter instead of writing unformatted text.
- Chained shortcuts from `runCommands` wait for Git commands in a selected submodule or nested repository.
- During a merge, rebase or cherry-pick, branch switching, stash, fetch, pull and push stay disabled.
- The mascot hops for Git commands in the selected repository, not only the folder's own.
- After switching folders, **Quick Open** lists the new folder's files without waiting for the old folder's listing.
- Files Git ignores show faint right after switching folders, without waiting for the old folder's read.
- Submodules and nested repositories are still found when **Initialize Repository** runs during a scan for them.
- **Running Plugins** and the **Plugins** settings page agree on the state of a plugin that is off or can't load.
- A language server that failed to start tries again when you open a file of its language 30 seconds later.
- Reloading, restarting or turning on a plugin starts again its language servers that failed to start.
- A language server that fails to start shows its error once, when it fails, instead of on every completion.

## 0.19.2

- With `terminalBackend` `"hc"`, terminals show up in the Herdr app, in a space for each folder.
- Terminals already running in herdr-core show up in Herdr too once Mayoi attaches them again.
- Set `terminalBackendHcShowInHerdr` to `false` to keep new terminals out of the Herdr app.
- Overwriting an SSH file that changed on its host no longer swaps the editor back to the host's old text.

## 0.19.1

- **Word Count** and **TODOs** come with Mayoi but start off; turn them on in **Settings → Plugins**.
- A plugin's `plugin.json` can say `"enabled": false` to start off until you turn it on.

## 0.19.0

- **Plugins → Show Running Plugins** opens a live table of each plugin's state, CPU time, memory and crashes.
- Click a column head in **Running Plugins** to sort by it; values refresh every second while the tab shows.
- Each row of **Running Plugins** can restart its plugin, turn it off, open its details, or show its log.
- **Plugins → Show Plugin Log** opens a live log that follows new lines, with a **Jump to latest** pill.
- Filter the **Plugin Log** by plugin, level and text, and use **Copy** and **Clear** in its bar.
- The **Plugin Log** keeps the last 10,000 lines and stays fast however long it gets.
- A plugin view's open folders and selection are now kept per window.
- Two lists in the same plugin view no longer show the same row as selected.
- Built-in status bar items that do nothing on click, such as the problem at the cursor, show their tooltip again.
- A plugin's settings now live only in its details; its row on **Settings → Plugins** links to them.
- Plugin sidebar views, such as **TODOs**, match the Explorer's rows, file icons, hover and selection.
- Plugin sidebar views show a loading state, a clear empty state, and fade in when their content arrives.
- Plugin status bar items, such as the word count, glide to a new width instead of shifting their neighbours.
- Plugin status bar items sink when pressed and show their tooltip even when they have no command.
- Plugin pick boxes look like the command palette, with its key hints and a shake when nothing matches.
- The command palette, pickers and prompts fade and scale in as they open, unless Reduce Motion is on.
- Click a plugin on the **Settings → Plugins** page to open its details; `Esc` goes back to the list.
- A plugin's details list its commands with their shortcuts, menus, status items, views, themes and more.
- A plugin's details show its memory, CPU time and start time, refreshed every second while open.
- A plugin's details show its recent log lines and its settings, with **Open Full Log** for the whole log.
- Plugin errors now show on the Plugins page as soon as they happen.
- The disabled **Install** button on the Plugins page now looks clearly disabled in dark themes.
- `mayoi plugin new` fills in `author` from your Git user name.
- **Find in Files** (`Shift+Cmd+F`) selects the previous query, so typing or pasting replaces it.
- Drag a file's tab onto a project in the rail to move it there; hold `Option` to copy it instead.
- Rest a dragged tab on a project in the rail to switch to it, then drop the tab where you want it.
- Unsaved changes come along when a tab moves to another project.
- Right-click a tab and choose **Move to Project** to send it to another project.
- New plugins: folders of JavaScript or TypeScript in `~/.mayoi/plugins`, which Mayoi runs with no Node or build step.
- Plugins add commands, shortcuts, menu items, status bar items, sidebar views, previews, themes, languages and snippets.
- `mayoi plugin install <git-url>` installs a plugin, and `list`, `update` and `remove` manage them.
- `mayoi plugin new <name>` writes a new plugin, and `mayoi plugin dev` reloads it each time you save.
- `mayoi plugin pack` copies the npm packages a plugin imports into it.
- The **Plugins** menu, and the **Plugins** page of the settings window, install, update, remove and configure plugins.
- **Plugins → Show Plugin Log** and **Show Running Plugins** show what plugins logged and how much they use.
- A plugin that loops or keeps too much memory is stopped, and `mayoi --disable-plugins` turns plugins off.
- The **Plugins** settings page shows each plugin's version, author, source, and a colored dot for its state.
- The **Plugins** page asks before it removes a plugin, and shows install and update errors on the plugin's row.
- While the plugin index loads, the **Plugins** page shows placeholder rows, and **Try Again** if it fails.
- Switches in the settings window respond to hover and stretch while held.
- A plugin's `plugin.json` and its index entry can name an `author`.
- The **Plugins** page shows no error while the plugin index isn't published yet.
- **Selection → Format Document** (⇧⌥F) formats with a plugin, and the `formatOnSave` setting formats as you save.
- Plugins underline problems in the editor; the status bar counts them, and **Show Problems** lists them.
- Plugins can bring language servers, for **Go to Definition** (F12), **Show Hover** and **Complete Word** (⌃Space).
- Plugins can add themes, including VS Code color themes as they are.
- **Selection → Insert Snippet…** inserts snippets that plugins bring, in VS Code's format.
- Each function a `.ts` file in `~/.mayoi/scripts` exports is a command; **Run Selection as Script** runs code once.
- Signed in, the plugins you installed from Git follow you to your other machines.
- New **Word Count** plugin shows the words of a Markdown file and its reading time in the status bar.
- New **TODOs** plugin lists the folder's TODO, FIXME and HACK comments in the sidebar.

## 0.18.2

- On Windows, right-click menus no longer cut off the end of their labels, as in Chinese.
- The Markdown preview highlights `==text==` and `<mark>` text with a yellow background.
- In Markdown, typing `=`, `*`, `_`, `~` or `$` over a selection wraps it; type it twice for `==` or `**`.
- The Markdown editor highlights `==text==` with a yellow background, as Obsidian does.
- The Markdown editor colors bold, italic and struck text between Chinese characters, as in `会**整体**线`.
- The Markdown editor shows `**bold**` in bold and draws a line through `~~struck~~` text.

## 0.18.1

- Opening a folder you switched away from no longer shows the terminals another window kept from it.
- On Windows, lines starting with tabs, like Go code, keep their indent, at four columns a tab.
- On macOS, Mayoi appears in Finder's **Open With** menu for text, Markdown and source code files.
- AI commit messages leave out the lines of more secret files, like `.envrc`, `*.tfvars` and `~/.aws` files.
- A changed `.env` or key file that is already committed no longer has its lines sent to the AI either.
- Keys, tokens and passwords in any changed line are swapped for `[left out]` before the diff goes to the AI.
- New `aiSecretFiles` setting adds your own patterns of files whose lines AI commit messages leave out.

## 0.18.0

- AI commit messages no longer send the contents of new secret files like `.env` or keys, only their names.
- With the `"hc"` backend, `terminalBackendHcPath` finds hc before `PATH` and `~/.local/bin`.
- With the `"hc"` backend, `terminalBackendHcEnv` passes `KEY=value` variables to hc's commands.
- With `terminalBackendHcPath` set, Mayoi no longer offers to install hc.
- `terminalBackendHcPath` and `terminalBackendHcEnv` stay on each machine and never upload with settings sync.
- Rename a terminal from its tab's right-click menu or **View: Rename Terminal…** in the command palette.
- A renamed terminal keeps its name when programs set a title; rename it to nothing to show theirs again.
- Pressing Enter in the crash report question chooses **Don't Send** instead of sending the report.
- On Windows, files indented with tabs, like Go code, show their indentation again.
- Find's match count changes instantly instead of rolling, in the editor, preview and terminal.
- The Git commit list and project rail use less CPU redrawing author and project initials.
- Windows can install `mayoi.cmd` from **File → Install 'mayoi' Command**, including user PATH setup.
- The host box of **Connect to Host…** has an **SSH Config** button that opens `~/.ssh/config` in a tab.
- **File → Remote SSH → Open SSH Config** and the command palette open it too, creating it when missing.
- Typing in Markdown with blank lines between paragraphs takes less CPU to keep the colors up to date.
- Terminals in tabs you can't see no longer redraw the window as they print, which saves CPU.
- Typing an absolute or `~/` path in Quick Open offers that file, even with no folder open.
- A path in Quick Open can end in `:line` or `:line:column`, and opens the file at that spot.
- Quoted or backslash-escaped paths pasted from a terminal open in Quick Open too.
- A project whose program works shows a turning ring at the corner of its square on the rail.
- When work finishes or fails while other work goes on, its dot shows inside the ring.
- The rail marks a project amber while it waits for you, green when done and red when work failed.
- A program waiting for you keeps its project's mark until it stops waiting, even after you look.
- Hovering a project on the rail lists what its programs wait for and work at, with their progress.
- The work ring on a rail square turns smoothly and fills to show progress when programs report it.
- A rail square's ring and dot grow in and fade out, and the dot blends from one state's color to the next.
- The rail tip marks waiting, finished and failed work with icons and shows progress as a small bar.
- A rail square's progress ring keeps its fill as it fades out, and completes when the work finishes.
- A rail square's dot sends out one ring in its color when work finishes or fails.

## 0.17.0

- Terminal icons show whether a program says it works, waits for you, finished or failed (OSC 7501).
- While a program works or waits for you, a pill over its terminal says what it does, with its progress.
- A program in a project out of sight that waits for you, finishes or fails marks the project on the rail.
- Typing in a terminal clears the mark of a program's finished or failed work.
- The program status pill shows the program's name in bold, its message, and its progress as a bar.
- When a program waits for you, its pill gets a colored ring and its icon pulses in time with the badge.
- As shown work finishes or fails, the pill says so with a check or a cross for a moment.
- The pill counts the program's other active tasks, and the terminal icon's tooltip lists them.
- A program that reports its status very often no longer slows the window down.
- **Rename Active File…** in the command palette renames the current tab's file, bindable as `file.rename`.
- Opening a file from outside the folder clears the selection in the Explorer.
- Splitting a Markdown preview to the right keeps it a preview, scrolled to the same place.
- **New File…** and the other items of the Explorer's menu below the files no longer do nothing at times.
- A plain mouse wheel scrolls the editor tabs sideways when more of them are open than fit.

## 0.16.2

- **View → Theme → The Matrix** puts the code's green inside steel chrome, like the film's real world.
- The Matrix gives each kind of code its own hue, and shows comments in steel that reads at 6.9:1.
- Under The Matrix, primary buttons are the red pill, chosen rows the blue, and Discard is orange.
- The White Rabbit is The Matrix's character on the welcome page and in the status bar, hopping while Git runs.
- The Matrix's digital rain is mirrored, as in the film, with a dimmer far layer behind it.
- Quick open, the command palette, pickers and dialogs now show at once instead of fading in.
- The find box, the sidebar and the panel appear instantly when opened with their shortcuts.
- Switching tabs or sidebar views no longer slides or fades the marks and the view.
- The selected row in quick open and pickers moves at once with the arrow keys.
- Vim's mode badge changes color at once on `i` and `Esc`.
- The theme picker shows all its rows at once, and moving through it switches the theme without a fade.
- Settings pages now fade in quickly without rising, and pressed buttons sink a little less.
- Mermaid diagrams copy as an image from their hover buttons or the opened diagram's header.
- Mermaid diagrams export as PNG or SVG from their export button, in the colors of the current theme.
- `.mmd` previews copy and export their diagram with **Copy Mermaid Diagram as Image** and the export commands.
- Pinching a touchpad zooms images, SVG and Mermaid previews, and opened diagrams around the pointer.
- On Windows, Ctrl with two-finger scrolling now zooms a picture instead of doing nothing.
- **View → Theme → Claude** now looks like Claude Code in a terminal, with the whole window in the code's font.
- Under Claude, the status bar shows Claude Code's ✻ spinner, the work's name and its seconds while Git runs.
- Claude's errors and removed lines are rose now, apart from its orange, and chosen rows are periwinkle.
- In Claude Light, the terminal's white and bright white text is readable on the light background.
- **View → Compact Folders** shows chains of single-child folders on one Explorer row.
- The Explorer header's context menu, the settings window and `explorerCompactFolders` toggle it too.
- Hover a compact row to see its whole path when the sidebar cuts it off.
- `⌘F` in a terminal opens a find bar that searches its output and scrollback, highlighting each match.
- In the terminal's find bar, `Return` goes to the match above and `Shift+Return` to the one below.
- On Linux and Windows, `Ctrl+F` in a terminal opens find instead of reaching the shell.
- Under Purple Gold, the welcome page drops the mosaic apse and arch and matches the other themes.
- In the find bar, `Shift+Return` now goes to the previous match instead of the next one.
- In the find bar's replace field, `Shift+Return` replaces the match and goes back to the one before it.
- In **Search**, `Shift+Return` in the query selects and previews the previous result, wrapping to the last.
- The Qoder theme lays the editors and terminal on a rounded card, in a grained shell without dividing lines.
- In the Qoder theme, dragging a sidebar or panel edge lights the edge only around the pointer.
- The Qoder theme titles the sidebar, Source Control and the terminal in sentence case, as Explorer.
- The Qoder theme colors code in Qoder's own chart colors: green, blue, violet, amber and rose.
- In the Qoder theme, Qoduck holds up a magnifier while searching and a laptop while Git works.
- Qoduck wears a headset while AI writes a commit message, and raises a megaphone when a toast appears.

## 0.16.1

- **View → Theme → Purple Gold** is now Porphyry: Tyrian purple chrome, gold leaf and gold hairlines.
- Under Purple Gold, a gold mosaic apse shimmers behind the welcome page, under an arched card.
- Purple Gold has a character, Corona, a jeweled crown that blinks, glances and hops while Git works.
- Purple Gold's code is in amethyst, gold, rose quartz and jade, and the open tab gets a gilded line.

## 0.16.0

- Turn on `editorAgentContext` to show a gutter **+** in ordinary files that collects code with a comment.
- Leave the comment empty and choose **Add to Agent** to collect the code alone.
- Code collected across files goes to the last terminal together with **Send N Items**.
- The **…** button above the editor lists collected items to go to, edit or remove; they survive restarts.
- **Mayoi → Settings…** (⌘,) opens a settings window with every preference by category, and a search.
- Changes in the settings window apply to every window at once, and each one resets with one click.
- **File → Keyboard Shortcuts…** lists every command's keys; double-click one and press its new keys to change them.
- The shortcuts editor warns when other commands use the keys, and keeps the comments of `keybindings.json`.
- A folder can set preferences such as `vim` or `autoSave` for itself in `.mayoi/settings.json`.
- Pick a folder at the top of the settings window to change its own settings rather than yours.
- `settings.json` and `keybindings.json` still open as files from **File → Open Settings (JSON)** and the like.
- **View → Theme** adds three Halloween themes, always dark: Jack-o'-Lantern, Witch's Brew and Blood Moon.
- Jack-o'-Lantern is pumpkin orange by candlelight, with a carved pumpkin on the welcome page and status bar.
- Witch's Brew is plum dark with slime green, and a bubbling cauldron for its character.
- Blood Moon is crimson and old gold on a red-tinged black, with a bat that flaps its wings as it hops.
- The settings window groups each section's settings in cards, and marks the ones you changed.
- The settings sidebar shows how many settings you have changed in each section.
- **Keyboard Shortcuts** shows each key as its own cap, and tags your own keys and removed ones.
- The dialog for new keys shows them large, and turns amber when another command has them.
- **Settings → Appearance** shows every theme as a small preview window; click one to switch to it.
- Font size, weight, line height and window zoom have sliders that show their value as you drag.
- **Settings → Fonts** shows sample code in your font, and it updates as you change the font settings.
- Settings with two or three short choices, such as `appearance`, are a row of buttons instead of a menu.
- Resetting a setting or a command's keys shows a notice with **Undo** for a few seconds.
- **Keyboard Shortcuts** groups commands by menu, and each menu's name stays at the top as you scroll.
- Filter the shortcuts to **All**, **Changed by Me** or **Conflicts**, each with how many it shows.
- Keys that two commands share show in amber with a warning sign in the list itself.
- In the shortcuts list, arrows choose a command, `Enter` changes its keys and `Delete` removes them.
- `⌘F` in the settings window goes to its search, and `↓` goes on from the search to the list.
- The arrow keys switch settings sections, and `→` or `Enter` moves into the section shown.
- **Settings → Fonts** picks the font from your installed monospaced fonts, each shown in itself.
- The font sample previews the font you point at or arrow to in the list, before you choose it.
- Text settings, such as the font, apply when you press `Enter` or leave the field, not at each key.
- A folder's own setting changed in the settings window no longer, now and then, snaps back.
- Headings in the Markdown preview take the color theme's heading color, as the editor does.
- Command-clicking a terminal path or URL followed by Chinese punctuation no longer takes the text after it.
- Switching back to a tab no longer flashes a blank editor for a frame before its text shows.
- When a file changes on disk, the editor stays where you were reading instead of jumping to the caret.
- The caret stays on its line when a file changes on disk above it, rather than drifting to other text.
- Select All no longer flashes the end of the file before the view settles.
- With word wrap on, switching back to a tab returns to the same lines, not lines nearby.
- Switching back to a tab whose caret is out of view no longer flashes a blank editor.
- With word wrap on, Enter, Tab, undo and redo no longer move the text around the screen.
- Pressing Enter or moving a line at the edge of the editor scrolls one line, not a third of the page.
- Clicking or holding near the right edge of a long line no longer runs the text sideways.
- Find scrolls a match at the top of the editor out from under the find bar.
- Deleting the longest line shrinks how far the editor scrolls sideways.
- A tab you switch back to shows the same lines after edits in another column added lines above them.
- With word wrap on, moving a tab to another column keeps the same lines in view.
- The folder button by the branch in source control opens **Switch Worktree…** to change checkouts.
- **Switch Worktree…** finds a checkout by folder or branch; Enter on a new name makes a worktree.
- Typing an existing branch's name in **Switch Worktree…** points to **Switch Branch…** instead.
- **Switch Branch…** lists local branches first, with each branch's last author and time.

## 0.15.0

- The Homebrew install steps add `brew trust sorrycc/mayoi`, which recent Homebrew needs to load the cask.
- Set `terminalBackend` to `"hc"` to run terminals in herdr-core, where shells outlive Mayoi's windows.
- Closing a terminal ends its shell in herdr-core, and `Ctrl+\` goes to the shell rather than detaching.
- Terminals in herdr-core attach again when Mayoi starts, with their scrollback, in their tabs and the panel.
- In a remote folder, terminals in herdr-core run on the host as one of hc's machines.
- Mayoi asks once before adding a host to hc, and its terminals use ssh until then.
- **View → Install hc…** installs or updates `hc` in `~/.local/bin` with herdr's installer, on macOS and Linux.
- With `terminalBackend` `"hc"` and no `hc` installed, the first new terminal offers to install it.
- Mayoi finds `hc` in `~/.local/bin` even when your shell's PATH lacks it, so it works right after installing.
- New terminals in herdr-core open at once and say they are starting, rather than freezing the window.
- A dot on a terminal's tab and panel chip shows that its shell runs in herdr-core; hover it for its address.
- The dot pulses, and a pill over the terminal says what it waits for, while it starts or its machine is away.
- The dot is cut out of the terminal icon by a thin ring, and its color fades as the terminal's state changes.
- A failed add to hc says why, suggesting a login only when hc asks for a password.
- When hc can't run on a host, Mayoi suggests installing it there and asks again on the next terminal.
- Once a terminal reaches herdr-core, its dot flashes and the pill shows a check and **Running in herdr-core**.
- A terminal fades while it waits for its machine, and comes back to full brightness once reattached.
- Hover a terminal's dot while it starts or waits to see what it waits for.
- The status bar shows a spinner while hc installs or a host is added to hc.
- A terminal stays faded while its shell starts in herdr-core, and brightens once it attaches.
- A terminal in herdr-core whose machine is away says its shell runs on there, rather than that it restarts.
- A waiting terminal's dot and spinner settle after a few seconds, so a long wait doesn't keep redrawing.
- The status bar shows a spinner while hc installs or a host is added to hc.
- A terminal stays faded while its shell starts in herdr-core, and brightens once it attaches.
- Mayoi starts faster with terminals of other machines to attach, and waits for a machine that is away.
- What you type in a terminal while it starts in herdr-core reaches the shell once it attaches.
- **Copy hc Address** in a terminal's menu copies its address in herdr-core, for `hc attach` elsewhere.
- The panel's terminals in herdr-core attach again when Mayoi starts, rather than starting new shells.

## 0.14.3

- Mermaid diagrams in a Markdown preview open over the window from their hover button or a double-click.
- An opened diagram zooms with its `−`/`+` buttons or `Cmd`-scroll, and drags to move; `Esc` closes it.
- Bold and italic in a Markdown preview render when CJK punctuation sits next to Latin text.
- Mayoi no longer crashes drawing a terminal after running for hours.
- Crash reports include what Mayoi printed before it crashed, so aborts from macOS can be tracked down.

## 0.14.2

- The color theme box opens scrolled to the theme you're using.
- `Cmd+A` in the editor selects all without scrolling to the end of the file.
- A branch with a GitHub pull request shows `#123`, its checks and review in Source Control and the status bar.
- Click the pull request badge to see its changes in a tab; right-click it for **Open on GitHub**.
- **Git → Open Pull Request…** lists open pull requests, or opens any one by its number; it needs `gh` signed in.
- A pull request tab shows the same changes as GitHub, inline or side by side, with its state and branches on top.
- Check **Viewed** on a file of a pull request to fold it and mark it viewed on GitHub, as its own checkbox does.
- When new commits reach the pull request, its tab offers **Reload** to show them.
- Commits that head an open pull request show a `#123` chip in the history; click it to open its changes.
- **Git → Show Pull Request Checks…** lists the CI checks of the branch's pull request and how each went.
- Click the badge's checks icon, or right-click the badge for **Show Checks…**, to open the same list.
- **Send to Terminal** on a failed job pastes the end of its log into the last terminal, without pressing Enter.
- **Re-run** runs a failed job again; **Copy Log** and **Open on GitHub** are there too.
- **Git → Open File on Remote** opens the file on GitHub, GitLab or Bitbucket at the caret line or selected lines.
- **Git → Copy Permalink** copies a link to the selected lines at the current commit.
- Cmd-click or Shift-click commits in **Source Control** to select several; Shift+arrow keys extend the range.
- Press `Cmd+C` in the commits list to copy the short hashes of the selected commits, one per line.
- Both are also in the tab and Explorer file context menus.
- **Stash Changes…** and **Stash Including Untracked…** in the Source Control **⋯** menu stash your changes.
- The **⋯** menu's **Stashes** submenu applies, pops or drops one of the last ten stashes; dropping asks first.
- **Git → Stashes…** lists every stash to search, with ⏎ to apply, ⌘⏎ to pop and ⌘⌫ to drop.
- Tall Mermaid diagrams in a preview show their boxes and arrows, not just their labels.

## 0.14.1

- **View → Theme → Purple Gold** is always dark: a near-black sidebar and a royal purple status bar.
- The Purple Gold theme uses classic gold for buttons, chosen rows and highlights, and golds and lavenders for code.
- The Cursor theme's current line, word highlights, find matches and hovered rows now match Cursor's own.
- Commit tooltips in the history no longer pop up over the rows while you scroll.
- Right-click a folder in **Explorer** and choose **Find in Folder…** to search only that folder.
- Bold, italic and strikethrough now render in the Markdown preview when quotes or brackets touch Chinese text.
- Vue files keep their colors after a directive value that prettier wraps over several lines.

## 0.14.0

- **View → Theme → Cursor** takes Cursor's own Cursor Light and Cursor Dark, as the Appearance is.
- The Cursor theme colors the code and the terminal as Cursor does, with its blue for buttons.
- Press `⌘⏎` in quick open to open the chosen file in the column to the side, splitting one if needed.
- The Markdown preview shows `[[note]]` and `[[note|label]]` wiki links, and a click opens the note.
- Wiki links find their note next to the file, at the repository's root, or anywhere in it, as Obsidian does.
- `[[note#heading]]` opens the note at that heading, as do Markdown links to `other.md#heading`.
- `![[image.png]]` shows the image in the Markdown preview, and `![[image.png|300]]` sets its width.
- `⌘`-click a `[[wiki link]]` in a Markdown file's text to open its note.
- The find bar's match count shows in full in every language, even with thousands of matches.
- In Chinese, the find bar's match count reads `1/5`, as in Japanese.

## 0.13.1

- When an update check fails, the update window shows the whole error, with no tooltip over it.
- The panes of a merge wrap long lines when **Word Wrap** is on, and `⌥Z` toggles wrapping for that merge tab.
- Conflict marks, gutter buttons and synced scrolling in a merge follow wrapped rows.
- A conflict taller than its pane keeps its gutter buttons in view while any of it shows.
- A sideways swipe or `Shift`+wheel scrolls the result pane of a merge, as it does the side panes.
- `⌘Z`, `⇧⌘Z`, `⌘X`, `Tab` and `Enter` show their edit at once, without the line highlight flickering.
- Undo and other edits no longer recenter the view when the caret is near its bottom edge.
- With **Word Wrap** on, undo and redo keep the editor where it is scrolled instead of jumping.

## 0.13.0

- **Close Other Projects** asks before closing the window's other projects.
- `⌘F` in a Markdown preview finds text in the rendered page, without switching to the source.
- Matches in a Markdown preview are highlighted, and `⌘G` and `⇧⌘G` scroll to each one.
- Typing and scrolling are smoother in files of Chinese, Japanese or other non-ASCII text.
- Window zoom scales the whole interface, including sidebars and terminals.
- Use `Cmd` on macOS or `Ctrl` on Windows/Linux with `=`, `-` or `0` to zoom or reset.
- Set `zoomLevel` in `settings.json` to save a window zoom level, including decimals.
- The command palette has separate commands for editor font size and image zoom.
- `⌘=` / `⌘-` / `⌘0` now zoom the whole window; editor font size and image zoom moved to the command palette.
- `⌘=` and `⌘-` no longer grow the editor font or zoom an image with focus; bind `editor.fontZoomIn` or `image.zoomIn` in `keybindings.json` for the old behaviour.
- Going to a line, a symbol or a search result briefly lights the line the caret lands on.
- The sidebar and the terminal panel fade in as they open.
- A folder's files fade in as it expands in the Explorer.
- The breadcrumbs' folder box and the account panel ease in, and project tips fade in.
- Typing in long files and scrolling diffs do less work each frame.
- Rows fade in as a group, a folder or a commit opens in **Source Control**, and as files change.
- A file's matches fade in as it unfolds in the search results.
- Scrolling a file of over 6,000 lines colors the lines ahead before they come into view.
- Typing recolors the edited lines with less work, and windows with many tabs draw with less.
- File tabs gain a **Copy Relative Path** option in their context menu.
- **View → Preview Markdown on Open** opens Markdown and `.mdx` in preview mode.
- `⇧⌘V` switches back to the source; new files, search results and line jumps always open as source.
- Restored and reopened Markdown tabs keep their view and scroll position, including offline remote tabs.
- MDX previews omit top-level `import`/`export` blocks, preserving code examples and paragraph text.
- Files dragged from **Explorer**, **Source Control** or **Search**, and folders from **Explorer**, drop into other apps.
- A remote file or folder dragged to another app arrives as its `host:/path` text.
- **File > Install 'mayoi' Command** uses a folder on your shell's `PATH`, or says how to add one.
- Uninstalling removes `mayoi` from every folder it was installed in.
- Install and upgrade on macOS with Homebrew using the `sorrycc/mayoi` tap.
- Homebrew installs the `mayoi` shell command and removes it when uninstalling the app.
- The Homebrew `mayoi` command reports when Mayoi.app cannot be found.

## 0.12.3

- MyGo 0.7.1: Shift-click extends terminal selections, and Powerline's rounded caps fill the line.
- Mermaid nodes and subgraphs with a light `style` fill keep their labels readable in dark themes.

## 0.12.2

- **Source Control** shows a folder's repositories and submodules in one list that scrolls as a whole.
- Repository and change-group headers in **Source Control** stay pinned at the top while their files scroll.
- Each repository header in **Source Control** has its own branch, remote and **⋯** actions.
- A repository's arrow expands or collapses it; clicking its name shows its history.
- Drag the divider between changes and history in **Source Control** to resize them for the folder.
- Submodule rows in **Source Control** say whether the commit reference or the content changed.
- Command-clicking a file path in a local terminal opens it in the editor, even if Git ignores it.
- Terminal paths ending in `:line` or `:line:column` open with the caret at that spot.
- **Open Preview to the Side** opens the preview in the column beside the file instead of adding one.
- The Markdown preview shows footnotes like `[^1]` as raised numbers, with the notes listed at the end.
- Clicking a footnote's number scrolls to its note, and the note's `↩` scrolls back.

## 0.12.1

- A file with a line too long to show, such as a minified `.map`, no longer opens and freezes Mayoi.
- Such a file left open is not reopened at launch, so Mayoi starts again.
- The Markdown preview draws LaTeX math between `$` and `$` as formulas within the text.
- Math between `$$` and `$$`, or in a `math` code block, shows as a centered formula of its own.
- A formula Mayoi cannot draw shows its source in red, and a block of math also says why.
- `previewMath` set to `false` shows math as its source.
- The Markdown preview links bare URLs right after punctuation such as `：`, as GitHub does.

## 0.12.0

- The Markdown preview draws `mermaid` code blocks as diagrams, in the colors of the theme.
- Hover a diagram for **Show Source** and **Copy Code**; one that cannot be drawn says why.
- `.mmd` and `.mermaid` files preview as their diagram with ⇧⌘V, and zoom like an SVG.
- `previewMermaid` set to `false` shows Mermaid blocks as code.

## 0.11.12

- `mayoi run <id>[=<arg>]` runs any command of the window, and `mayoi commands` lists them.
- `mayoi -g file:line` opens a file at a line, `-d` compares two files, and `-n` opens a new window.
- `mayoi --remote host:/path` opens a folder on an SSH host.
- `mayoi -w` waits for its files to close, so it works as `git config core.editor "mayoi -w"`.
- `mayoi` says what went wrong and exits non-zero when it fails.
- Mayoi updates a `mayoi` command installed by an older version as it starts.
- Opened from the Finder or Dock, Mayoi uses your shell's `PATH`, so git hooks find `pnpm` and `node`.
- Fix a crash on **Go → Go to Symbol…** (⇧⌘O) in a file with a line that starts with `func() {`.

## 0.11.11

- The welcome page has a **Star on GitHub** link.
- Fix a crash on clicking a tab of another column while the find box is open.
- Fix a crash on dropping a tab or terminal from one window into another.
- Crash logs from before 0.11.6 are reported as of an unknown version.

## 0.11.10

- The diff toolbar's menus have their own icons, and the split toggle is gone.
- A commit's file panel is hidden until shown with **Git → Show Files of Commits**.
- **⌘F** and **⌥⌘F** select the find box's query, so typing replaces it.

## 0.11.9

- The Grok Bot theme, with Grok Bot on the welcome page and in the status bar.
- **View → Theme → Color Theme…** previews each theme as you move through the list.
- Changing the theme or appearance fades the colors, unless reduced motion is on.
- A commit's tab lists its changed files in a tree beside the diff.
- The arrow by a commit in **COMMITS** lists the files it changed.

## 0.11.8

- **View → New Terminals in Editor Area** opens new terminals as editor tabs.
- The Markdown preview renders HTML as GitHub does, including centered logos and sized images.
- The Markdown preview shows images from the web, such as badges.
- The vim mode badge switches to the new mode at once instead of fading.

## 0.11.7

- Untitled documents are kept on quit and reopen at the next launch.
- Branch switching, unstaging and discarding work with Git older than 2.23.
- Long paths in diff headers shorten to fit narrow columns.
- Files copied in the Finder paste into a terminal as quoted full paths.
- On macOS, holding a key repeats it instead of showing accents.
- Linux: tab stops and crisp text on Ubuntu 22.04 work again.

## 0.11.6

- After a crash, Mayoi offers to send an anonymous crash report.
- Terminal tabs stay clickable when the tab strip overflows.
- The merge view's bars move actions into a **⋯** menu as the window narrows.
- The status bar's branch opens the same branch picker as Source Control.
- A merge's three panes scroll together.
- The merge gutter's bands stay inside the gutter.
- Command-scrolling over a terminal no longer changes its font size.
- Linux and Windows: a title bar menu button replaces the menu bar.
- Light or Dark appearance holds when the desktop uses the other.
- Maximizing the bottom terminal panel fills the window again.
- MyGo 0.6.0: faster redraws, smooth wheel scrolling, Kitty graphics in the terminal.

## 0.11.5

- Discarding a diff block edits the file's document, as one undo step.
- Unstaged diff blocks have an arrow that reverts the block.
- Control-click the arrow to append the other version's lines instead.
- Edit the working tree side of a diff in place with **I** or **Edit**.
- Diff tabs have **Previous File** and **Next File** buttons and a **Side by Side** toggle.
- Change navigation stops at a file's ends before moving to the next file; see `diffCrossFiles`.
- Opening a file from a diff lands on the current change's line.
- Deleted files offer **Restore File**, and untracked files **Remove File…**.
- The **Whitespace** menu filters whitespace changes in diffs.
- **Highlight** and **Context** menus set word or character highlighting and context lines.
- **Synchronized Scrolling** in the Layout menu lets diff sides scroll apart.
- **Line Alignment** in the Layout menu, or `diffAlignedRows`, turns off row alignment.
- Diffs keep their place when folding, wrapping or resizing.
- **Git → Compare Two Files…** diffs any two local files.
- Conflicted files open in a three-way merge view.
- The merge view lists the files in conflict and applies non-conflicting changes automatically.
- Side-by-side diffs have a gutter of bands linking changes, as in JetBrains IDEs.
- Changed words are compared against the paired line, not the same line number.
- With **Line Alignment** off, both sides of a scrolled split diff stay visible.
- Change navigation glides, and change buttons fade with the pointer.
- The merge view shows full lines, scrolls its panes together, and has gutters with accept buttons.
- A Git sync button in the status bar shows incoming and outgoing commits.
- Linux: fix a crash when the menus change (#51).

## 0.11.4

- Mayoi checks for updates hourly and shows them in the status bar instead of opening a window.
- Mayoi signs in to Mon as the Mac's name and no longer piles up sign-ins.

## 0.11.3

- The Markdown preview's text can be selected and copied.
- A redesigned Markdown preview, closer to GitHub's.
- The Markdown preview shows GitHub alerts such as `[!NOTE]`.
- Links to headings, as `[Usage](#usage)`, scroll the preview.
- **Open Preview to the Side** shows a Markdown or SVG preview in the next column.
- An empty Markdown document's preview says there's nothing to preview.
- Diffs highlight changed words correctly below added or removed lines.
- The Markdown preview parses and draws faster.
- Fitted images and SVG previews cast a soft shadow.
- The Markdown preview scrolls with its text in another column, and the other way round.
- Switching between a Markdown file's text and preview keeps the place.
- Markdown tables are as wide as their text, with round corners.
- The Markdown preview's status bar shows the word count and reading time.
- Images the preview can't show appear as cards with their alt text.
- Heading spacing in the Markdown preview matches GitHub's.
- Images zoomed past their size show crisp pixels.

## 0.11.2

- New Terminal in Editor Area skips locked columns.

## 0.11.1

- Source Control lists the Git repositories inside a non-repository folder (#17).
- Windows: submodules are named with `/`.
- Fix dragging a project's square along the rail.

## 0.11.0

- Settings sync no longer undoes a change made while it syncs.
- Windows: many path fixes for quick open, rename, the Markdown preview and Source Control.
- Windows: terminals are named after their shell.
- `~` shortens only the home folder itself, not folders whose names start the same.
- The Windows fixes follow @wowyuarm's port (#40); tests now run on macOS and Windows.
- The Markdown preview's column is 120 characters wide instead of 80.
- Vim mode records and plays macros: `qa`, `@a`, `@@`.
- Vim mode has registers, `"a`–`"z` and the special ones; `:reg` shows them.
- Vim mode has marks and a jump list; `:marks` lists them.
- Vim mode adds `:normal`, `:g`, `:v`, `:m`, `:t`, `:pu`, `&` and `g&`.
- Vim mode adds `it`, `at`, `[(`, `[{`, `])`, `]}`, `gp` and `gP`.
- Vim mode repeats inserts with a count, as `3ix`.
- Vim mode: ⌃W and ⌃U delete in insert mode on macOS.
- Vim mode: `ma` sets a mark.
- Vim mode's search highlighting does less work per frame.
- Vim mode shows a half-block cursor while an operator waits, and flashes yanked text.
- Vim mode has `/` and `?` search with live highlighting and match counts.
- Vim mode's `:` runs commands such as `:w`, `:q`, `:s`, `:sort` and `:vs`.
- Vim mode adds `gv`, `gi`, `gJ`, `H M L`, `zz zt zb`, ⌃A, ⌃X, `ZZ` and `ZQ`.
- Vim mode matches vim in more edge cases, such as `2D`, `d}` and `di(`.
- Vim mode adds `ip`, `ap`, `3V`, `3v`, and `.` after visual changes.
- Vim mode: `p` over a selection yanks what it replaces.
- Esc after `o`, `O`, `S` or `cc` with nothing typed removes the indent.
- Vim mode's keys work on non-Latin keyboard layouts.
- Find, quick open and the command palette shake when Return finds nothing.
- The find box, change peek and review comment box slide up as they close.
- Find and search fields fade their focus border.
- Lists, buttons and the sync panel fade their hover colors and press down.
- Hover-only buttons fade in and out.
- Splitter lines show after the pointer rests on them.
- Empty-state badges pop in, and a commit that leaves nothing ends with a check.
- Switching sidebar views fades in.
- Source Control's group counts roll to their new numbers.
- Tabs and terminal chips animate as they open, close and move.
- Buttons, tabs, breadcrumbs, the status bar and the welcome page fade their hovers.
- The sidebar's view marker and the picker's selection slide.
- Chevrons rotate as sections open and close.
- Change counts and find's match count roll to their new numbers.
- The vim badge, branch name and title bar cross-fade as they change.
- The welcome page animates in.
- Returning to a wrapped file's tab keeps its scroll position.
- The title bar's box no longer leaves a gap after a short name.
- The welcome page's rows are all one height.
- The vim mode badge in the status bar is colored by mode.
- The command palette shows a ⌘ icon.
- The welcome page lists six recent folders, then **More…**.
- Source Control's message box grows with the message, up to eight lines.
- Cut with no selection cuts the whole line.
- Fix splitting a diff to the right closing the window (#20, #39).
- Quick open and the command palette rank consecutive letter matches higher.
- The Markdown preview underlines large headings and mutes quotes, as GitHub does.
- Quick Open has its own icon on the welcome page.
- Click a folder in the breadcrumbs to browse its files (#38).
- Light themes meet WCAG AA contrast for Git status colors and errors.
- A missing `fontFamily` or `terminalFontFamily` falls back to the system monospace font.
- Breadcrumbs of files outside the folder show `~` and no empty segment.
- Typing in long files re-highlights less.
- Source Control lists each submodule's changes.
- The ✨ button writes commit messages with AI when signed in to Mon.
- **Sign In to Sync Settings…** is now **Mayoi → Sign In…**.

## 0.10.0

- Settings sync: sign in to sync `settings.json` and `keybindings.json` between machines.
- Settings sync shows its status in the status bar and an account panel.
- `syncIgnored` keeps chosen settings on one machine.
- Settings sync retries on its own and explains errors.
- Conflicting `keybindings.json` changes open side by side to merge.
- Mayoi speaks Simplified Chinese and Japanese (#33).
- Commit comparisons open in a diff tab instead of a patch.
- Right-click the terminal panel's header to move, maximize or hide it.
- Source Control's **Remotes** shows where each remote points.
- Fix "No remotes" flashing when switching back to **Remotes**.
- Commits keep their time visible in a narrow sidebar.
- The terminal panel can sit at the bottom, left or right; see `panelPosition`.
- The Explorer and Source Control tree indent less.
- Linux: fix a startup crash on GPU machines (#24).
- Linux: text is crisp again with subpixel antialiasing.
- Linux: tabs indent to tab stops.
- Linux: submenus of the menu bar open correctly.
- Windows and Linux: shortcuts show as key caps.
- Linux: `install.sh` and the Debian package add the `mayoi` command.
- Opening a branch from another worktree works with Git before 2.36.
- Canceling a remote paste leaves nothing behind.
- Fix Git work from a previous folder landing in the new one's Source Control.
- Source Control's **Repository** section manages branches and remotes.
- Clicking anywhere on an Explorer row opens the file, and arrow keys preview files.
- Review comments: comment on diff or file lines and send them to the terminal.
- A **+** button in diffs adds a review comment.
- Diffs with review comments offer **Send N Comments**.
- Files show review comments after their lines.
- Redesigned review comment cards with an **Outdated** badge.
- Review comments use the window's font on a raised card.
- Review comments are kept with the session.
- `n` and `N` in a diff go between review comments.
- Deleting review comments can be undone.
- Review comments in files show on a tinted chip.
- Lock a column to keep it for terminals.
- Source Control supports multi-select with Shift and Cmd.
- The Explorer and Source Control tree draw indent guides.
- The Markdown preview draws task checkboxes and softer bullets.
- Fitted images have a margin and hairline border.
- Painting allocates about a quarter as much.
- Windows: background git no longer flashes console windows.
- MyGo 0.5.5: system double-click speed, and image paste in terminal programs.
- A commit's menu saves, copies, compares and opens it on GitHub, GitLab or Bitbucket.
- **File → New Window** (⇧⌘N) opens an empty window.
- macOS: drop files on the Dock icon, and use Mayoi from **Open With** (#28).
- Middle-click closes a tab.
- Fix a crash when files leave a scrolled diff.
- **Project Tabs** shows the rail with a single project too.
- The rail always shows project icons; **Project Icons** is gone.
- Remote folders show a server badge on the rail.
- Rail labels use two letters to tell similar names apart.
- **Rename Tab…** and **Tab Color** customize a project's square.
- Drop an image on a project's square, or use **Choose Icon…**, to set its icon.

## 0.9.1

- ⇧⌘E focuses the Explorer.
- Remote SSH: copy, paste, drag, upload and download files between machines.
- Remote SSH: forward ports, including from `localhost` links in a terminal.
- Remote SSH: Explorer folders open faster.
- Remote SSH: saving a file changed on the host asks before overwriting.
- Remote SSH: terminals survive a dropped connection and restart on reconnect.
- Remote SSH: pasting files or a screenshot into a remote terminal uploads them.
- Fix Source Control not showing right after `git init`.
- With Git 2.55+, submodule diffs show their commits again.
- Remote SSH: reconnects automatically after a drop.
- Remote SSH: Connect to Host groups recent hosts and shows progress.
- Remote SSH: the folder picker lists folders by name with `..` and `~`.
- Remote SSH: a disconnected folder's Explorer offers to reconnect.
- Remote SSH: fewer round trips and slower polling in the background.
- **View → Project Icons** shows each project's favicon or logo on the rail.
- Project Icons finds mygo, site, docs and macOS app icons.

## 0.9.0

- Remote SSH: **File → Remote SSH → Connect to Host…** opens a folder on another machine.
- Right-to-left selections extend from where they started.

## 0.8.2

- Selecting text in a terminal copies it; see `terminalCopyOnSelect`.
- Each project keeps its own sidebar and panel layout.
- ⌘V pastes a screenshot into Claude Code in the terminal.
- Fix new files in dot-folders, such as `.changeset`, in Changes.

## 0.8.1

- A redrawn project tab rail, with drag to reorder and folder drop to open.
- A terminal bell or notification marks its project's square.
- An empty folder's Explorer offers **New File…** and **New Folder…**.

## 0.8.0

- The title bar and status bar adapt to narrow windows.
- Inactive tabs no longer lay out every frame.
- **View → Project Tabs** keeps every folder in one window, with a rail.
- The commits menu can merge, rebase, reset, revert and cherry-pick.
- Author initials are centered in their circle.
- Crashes are logged to `crash.log` and the `crashes` folder.

## 0.7.0

- MyGo 0.4.8: shortcuts on non-Latin layouts and faster long files.
- The Qoder theme, with Qoduck on the welcome page and in the status bar.
- Switching to a branch in another worktree offers to open that worktree.
- Better Windows and Linux shortcuts, with VS Code's keys; see `vimCtrlKeys`.
- Custom shortcuts in `keybindings.json`, in VS Code's format.
- New commands to sort lines, camel case, copy or reveal the active file, and set indentation.
- The sidebar's title is no longer cut short.
- Quick open's footer shows its prefixes as keys.
- Destructive buttons such as **Discard** are a deeper red.
- **⇧⌘V** previews Markdown files.
- Sideways trackpad swipes scroll long lines smoothly.

## 0.6.0

- **⌘C** and **⌘V** copy and paste files in the Explorer.
- Double-clicking a file in the Explorer keeps its tab.
- Commits clicked in **COMMITS** open in the preview tab.
- Drag files from the Explorer, Source Control or Search to open them where dropped.
- A commit's tab wraps its message.
- Click the Explorer's empty space to deselect, or right-click it for a workspace menu.

## 0.5.1

- **COMMITS** draws the history as a graph with branches and tags.
- **Show All Branches** shows every branch and tag; see `scmAllBranches`.
- A commit's menu checks it out, creates a branch or tag, and copies names.
- Fix the source control **⋯** menu ignoring clicks past the sidebar.
- A closed folder reopens with its tabs, columns and Explorer state.
- Tabs restore their caret, selection, scroll and word wrap.
- Window state moves from `settings.json` to `state.json`.
- Fix a crash when closing a column's last tab.

## 0.5.0

- The Claude theme, with Clawd on the welcome page and in the status bar.
- **View → Theme** picks the theme and, separately, System, Light or Dark.
- **Search** (⇧⌘F) finds and replaces across files.
- Search results are keyboard-navigable and searches are faster.
- Preview tabs: a single-clicked file opens in an italic tab the next one replaces.
- **File → Open Settings** (⌘,) applies changes on save.
- New font settings, such as `fontWeight`, `fontFeatures`, `lineHeight` and `terminalFontFamily`.
- Zoom resizes terminals in every window.
- **Reveal in Explorer** and **Reveal in Finder** in a tab's menu.
- The welcome page's section titles match the sidebar's.
- Diffs of binary or unchanged files say so in the window's font.

## 0.4.0

- **Go → Command Palette…** (⇧⌘P) runs any command by name.
- Better language detection for `go.mod`, `.sql`, Brewfiles and `.npmrc`.
- The Matrix's welcome page sits on a panel so it reads.
- Line numbers and secondary text are easier to read.

## 0.3.3

- **View → Theme** adds The Matrix.
- Clicking a file or commit in source control focuses its changes.

## 0.3.2

- Drop files from the Finder to open them, or on a terminal to type their paths.
- Primary buttons stay purple on hover and press.
- Quick open, pickers, find and change peek fade in.
- A window without a folder offers to open one in the Explorer.

## 0.3.1

- MyGo 0.4.1: fix select lists jumping and input method highlighting.
- Release files are named `Mayoi.<version>.<arch>.dmg` and `Mayoi.Setup.<version>.<arch>.exe`.

## 0.3.0

- **View → Vim Mode** edits with vim's keys.
- **View → Sidebar on Right** moves the sidebar.

## 0.2.1

- Source control lists the branch's recent commits.
- The commit button says what it will do, and adds **Undo Last Commit**.
- **Switch Branch…** finds, switches and creates branches.
- **View as Tree** lists changes under their folders.
- A clean work tree says so and offers to push or pull.
- The commit summary shows its length past 50 characters.
- A progress bar and toasts show Git commands' progress.
- Stage, unstage or discard one change at a time.
- A file's header sticks to the top of the diff.
- Diff headers show icon, state, rename source and line counts.
- Source control stays fast with thousands of changes.
- **File → Open Recent** lists recent folders, and ⌃R finds one.

## 0.2.0

- Source control: stage, unstage, discard and commit.
- Diffs open in a tab, inline or side by side.
- Diffs fold unchanged lines and wrap long ones.
- One **Changes** tab shows every changed file.
- Pager keys move around a diff.
- Click a gutter mark to see, revert or stage a change.
- The Explorer shows Git status colors.
- The status bar's branch switches, fetches, pulls and pushes.
- The caret's line shows who changed it last.
- ⇧⌘T reopens the last closed tab.
- Highlighting keeps up with typing in large files.
- Large files highlight the visible lines first.
- The tab strip has a bottom edge.
- Readability tweaks to line numbers, find fields and the status bar.

## 0.1.0

- The first release: tabs, columns, file tree, quick open, find, Git marks, terminal and auto save.
- Mayoi updates itself from GitHub Releases.
