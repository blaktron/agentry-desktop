package codingtools

// The common places a command-line tool's executable lands when PATH does
// not name it: Homebrew on Apple silicon and on Intel (and its Linux
// prefix), a user's npm prefix, and the directories a vendor's install
// script picks. Each entry below lists the ones its tool uses.
const (
	brewArm   = "/opt/homebrew/bin/"
	brewIntel = "/usr/local/bin/"
	linuxbrew = "/home/linuxbrew/.linuxbrew/bin/"
	npmUser   = "~/.npm-global/bin/"
	npmWin    = "${APPDATA}/npm/"
	userBin   = "~/.local/bin/"
)

// unixBins is name in the shared directories of macOS and Linux.
func unixBins(name string) []string {
	return []string{userBin + name, brewArm + name, brewIntel + name, linuxbrew + name, npmUser + name, "/usr/bin/" + name}
}

// The markers that say a tool holds the plugin, by the text of the file
// that lists it.
const (
	claudeMarker = `"agentry@agentry"`
)

// catalog is every coding tool the app knows, in the order the list shows
// them: the tools the runner drives first, then their apps, then the rest
// by how many people use them. The install commands are the ones each
// plugin package's README gives, with the flags that keep them from asking
// on a terminal the app does not have.
var catalog = []Tool{
	{
		ID: "claude-code", Name: "Claude Code", Kind: KindCLI,
		Site:     "https://code.claude.com/docs/en/setup",
		Commands: []string{"claude"},
		Locations: Paths{
			"windows": {"~/.local/bin/claude.exe", npmWin + "claude.cmd"},
			"*":       unixBins("claude"),
		},
		Package: "claude-code",
		Steps: [][]string{
			{"plugin", "marketplace", "add", "blaktron/agentry-plugin-claude-code"},
			{"plugin", "install", "agentry@agentry", "--scope", "user"},
		},
		Repeatable: true,
		Markers:    Paths{"*": {"${CLAUDE_CONFIG_DIR}/plugins/installed_plugins.json", "~/.claude/plugins/installed_plugins.json"}},
		MarkerText: claudeMarker,
	},
	{
		ID: "claude-app", Name: "Claude", Kind: KindApp,
		Site:     "https://claude.com/download",
		Commands: []string{"claude-desktop"},
		Locations: Paths{
			"darwin":  {"/Applications/Claude.app", "~/Applications/Claude.app"},
			"windows": {"${LOCALAPPDATA}/AnthropicClaude/claude.exe", "${LOCALAPPDATA}/Packages/Claude_pzs8sxrjxfjjc"},
			// The build of Claude Code the app's Code tab runs on this
			// computer, kept beside the plugins it reads.
			"*": {"~/.claude/remote/ccd-cli/*"},
		},
		Package:    "claude-code",
		Installer:  "claude-code",
		Bundled:    Paths{"*": {"~/.claude/remote/ccd-cli/*"}},
		Steps:      [][]string{{"plugin", "marketplace", "add", "blaktron/agentry-plugin-claude-code"}, {"plugin", "install", "agentry@agentry", "--scope", "user"}},
		Repeatable: true,
		Markers:    Paths{"*": {"${CLAUDE_CONFIG_DIR}/plugins/installed_plugins.json", "~/.claude/plugins/installed_plugins.json"}},
		MarkerText: claudeMarker,
		Note:       "The app's Code tab reads the same plugins as Claude Code, so one install serves both.",
	},
	{
		ID: "codex", Name: "Codex", Kind: KindCLI,
		Site:     "https://developers.openai.com/codex/cli",
		Commands: []string{"codex"},
		Locations: Paths{
			"windows": {"${LOCALAPPDATA}/Programs/OpenAI/Codex/bin/codex.exe", "~/.local/bin/codex.exe", "~/.codex/packages/standalone/current/bin/codex.exe", npmWin + "codex.cmd"},
			"*":       append(unixBins("codex"), "~/.codex/packages/standalone/current/bin/codex"),
		},
		Package: "codex",
		Steps: [][]string{
			{"plugin", "marketplace", "add", "blaktron/agentry-plugin-codex"},
			{"plugin", "add", "agentry@agentry"},
		},
		Repeatable: true,
		Markers:    Paths{"*": {"${CODEX_HOME}/config.toml", "~/.codex/config.toml"}},
		MarkerText: claudeMarker,
	},
	{
		ID: "codex-app", Name: "Codex app", Kind: KindApp,
		Site: "https://developers.openai.com/codex/app",
		Locations: Paths{
			"darwin": {"/Applications/Codex.app", "~/Applications/Codex.app"},
		},
		Package:    "codex",
		Installer:  "codex",
		Bundled:    Paths{"darwin": {"/Applications/Codex.app/Contents/Resources/bin/codex", "~/Applications/Codex.app/Contents/Resources/bin/codex"}},
		Steps:      [][]string{{"plugin", "marketplace", "add", "blaktron/agentry-plugin-codex"}, {"plugin", "add", "agentry@agentry"}},
		Repeatable: true,
		Markers:    Paths{"*": {"${CODEX_HOME}/config.toml", "~/.codex/config.toml"}},
		MarkerText: claudeMarker,
		Note:       "The app reads the same plugins as the codex command, so one install serves both.",
	},
	{
		ID: "opencode", Name: "OpenCode", Kind: KindCLI,
		Site:     "https://opencode.ai",
		Commands: []string{"opencode"},
		Locations: Paths{
			"windows": {"~/.opencode/bin/opencode.exe", npmWin + "opencode.cmd"},
			"*":       append([]string{"~/.opencode/bin/opencode"}, unixBins("opencode")...),
		},
		Package:    "opencode",
		Steps:      [][]string{{"plugin", "add", "github:blaktron/agentry-plugin-opencode"}},
		Repeatable: true,
		Markers: Paths{"*": {
			"${XDG_CONFIG_HOME}/opencode/opencode.json", "${XDG_CONFIG_HOME}/opencode/opencode.jsonc",
			"~/.config/opencode/opencode.json", "~/.config/opencode/opencode.jsonc",
		}},
		MarkerText: "blaktron/agentry-plugin-opencode",
		Note:       "The plugin needs OpenCode 2 or later.",
	},
	{
		ID: "opencode-app", Name: "OpenCode app", Kind: KindApp,
		Site: "https://opencode.ai/download",
		Locations: Paths{
			"darwin":  {"/Applications/OpenCode.app", "~/Applications/OpenCode.app"},
			"windows": {"${LOCALAPPDATA}/Programs/OpenCode/OpenCode.exe"},
			"linux":   {"/usr/share/applications/ai.opencode.desktop.desktop", "~/.local/share/applications/ai.opencode.desktop.desktop"},
		},
		Package:    "opencode",
		Installer:  "opencode",
		Steps:      [][]string{{"plugin", "add", "github:blaktron/agentry-plugin-opencode"}},
		Repeatable: true,
		Markers: Paths{"*": {
			"${XDG_CONFIG_HOME}/opencode/opencode.json", "${XDG_CONFIG_HOME}/opencode/opencode.jsonc",
			"~/.config/opencode/opencode.json", "~/.config/opencode/opencode.jsonc",
		}},
		MarkerText: "blaktron/agentry-plugin-opencode",
		Note:       "The app loads the plugins in OpenCode's global configuration, which the opencode command writes.",
	},
	{
		ID: "pi", Name: "pi", Kind: KindCLI,
		Site:     "https://pi.dev",
		Commands: []string{"pi"},
		Locations: Paths{
			"windows": {"~/.pi/agent/bin/pi.cmd", npmWin + "pi.cmd"},
			"*":       append([]string{"~/.pi/agent/bin/pi"}, unixBins("pi")...),
		},
		Package:    "pi",
		Steps:      [][]string{{"install", "git:github.com/blaktron/agentry-plugin-pi"}},
		Markers:    Paths{"*": {"${PI_CODING_AGENT_DIR}/settings.json", "~/.pi/agent/settings.json"}},
		MarkerText: "blaktron/agentry-plugin-pi",
	},
	{
		ID: "qwen", Name: "Qwen Code", Kind: KindCLI,
		Site:     "https://qwenlm.github.io/qwen-code-docs/",
		Commands: []string{"qwen"},
		Locations: Paths{
			"windows": {npmWin + "qwen.cmd"},
			"*":       unixBins("qwen"),
		},
		Package: "claude-code",
		Steps:   [][]string{{"extensions", "install", "https://github.com/blaktron/agentry-plugin-claude-code:agentry", "--consent"}},
		Markers: Paths{"*": {"~/.qwen/extensions/agentry/qwen-extension.json"}},
		Note:    "Qwen Code installs the Claude Code package.",
	},
	{
		ID: "gemini-cli", Name: "Gemini CLI", Kind: KindCLI,
		Site:     "https://github.com/google-gemini/gemini-cli",
		Commands: []string{"gemini"},
		Locations: Paths{
			"windows": {npmWin + "gemini.cmd"},
			"*":       unixBins("gemini"),
		},
		Package: "gemini-cli",
		Steps:   [][]string{{"extensions", "install", "https://github.com/blaktron/agentry-plugin-gemini-cli", "--consent", "--skip-settings"}},
		Markers: Paths{"*": {"~/.gemini/extensions/agentry/gemini-extension.json"}},
	},
	{
		ID: "copilot", Name: "GitHub Copilot CLI", Kind: KindCLI,
		Site:     "https://github.com/github/copilot-cli",
		Commands: []string{"copilot"},
		Locations: Paths{
			"windows": {"${LOCALAPPDATA}/Microsoft/WinGet/Links/copilot.exe", npmWin + "copilot.cmd"},
			"*":       unixBins("copilot"),
		},
		Package: "agent-plugins",
		Steps:   [][]string{{"plugin", "install", "blaktron/agentry-plugin-agent-plugins"}},
	},
	{
		ID: "antigravity", Name: "Google Antigravity", Kind: KindApp,
		Site: "https://antigravity.google",
		Locations: Paths{
			"darwin": {"/Applications/Antigravity.app", "~/Applications/Antigravity.app", "/Applications/Antigravity IDE.app", "~/Applications/Antigravity IDE.app"},
		},
		Package:   "antigravity",
		Installer: "agy",
		Steps:     [][]string{{"plugin", "install", "https://github.com/blaktron/agentry-plugin-antigravity"}},
		Markers:   Paths{"*": {"~/.gemini/config/plugins/agentry"}},
		Note:      "The app, the editor and the agy command share their plugins, so one install serves all three.",
	},
	{
		ID: "agy", Name: "Antigravity CLI", Kind: KindCLI,
		Site:     "https://antigravity.google/cli",
		Commands: []string{"agy"},
		Locations: Paths{
			"windows": {"${LOCALAPPDATA}/agy/bin/agy.exe"},
			"*":       unixBins("agy"),
		},
		Package: "antigravity",
		Steps:   [][]string{{"plugin", "install", "https://github.com/blaktron/agentry-plugin-antigravity"}},
		Markers: Paths{"*": {"~/.gemini/config/plugins/agentry"}},
	},
	{
		ID: "droid", Name: "Factory Droid", Kind: KindCLI,
		Site:     "https://docs.factory.ai/cli/getting-started/quickstart",
		Commands: []string{"droid"},
		Locations: Paths{
			"windows": {"~/bin/droid.exe"},
			"*":       unixBins("droid"),
		},
		Package: "claude-code",
		Steps: [][]string{
			{"plugin", "marketplace", "add", "blaktron/agentry-plugin-claude-code"},
			{"plugin", "install", "agentry@agentry", "--scope", "user"},
		},
		Markers:    Paths{"*": {"~/.factory/settings.json"}},
		MarkerText: claudeMarker,
		Note:       "Droid installs the Claude Code package.",
	},
	{
		ID: "hermes", Name: "Hermes Agent", Kind: KindCLI,
		Site:     "https://hermes-agent.nousresearch.com",
		Commands: []string{"hermes"},
		Locations: Paths{
			"windows": {"${LOCALAPPDATA}/hermes/bin/hermes.exe"},
			"*":       unixBins("hermes"),
		},
		Package: "agent-plugins",
		Steps:   [][]string{{"plugins", "install", "blaktron/agentry-plugin-agent-plugins", "--enable"}},
	},
	{
		ID: "openclaw", Name: "OpenClaw", Kind: KindCLI,
		Site:     "https://openclaw.ai",
		Commands: []string{"openclaw"},
		Locations: Paths{
			"windows": {npmWin + "openclaw.cmd"},
			"*":       unixBins("openclaw"),
		},
		Package: "openclaw",
		// --accept-capabilities is the consent OpenClaw otherwise asks on a
		// terminal; the app's own dialog, which names this command, asks it.
		Steps: [][]string{{"plugins", "install", "clawhub:@secureagentry/openclaw-agentry", "--accept-capabilities"}},
	},
	{
		ID: "openclaw-app", Name: "OpenClaw app", Kind: KindApp,
		Site: "https://openclaw.ai",
		Locations: Paths{
			"darwin": {"/Applications/OpenClaw.app", "~/Applications/OpenClaw.app"},
		},
		Package:   "openclaw",
		Installer: "openclaw",
		Steps:     [][]string{{"plugins", "install", "clawhub:@secureagentry/openclaw-agentry", "--accept-capabilities"}},
		Note:      "The app runs the same gateway as the openclaw command, so one install serves both.",
	},
	{
		ID: "devin", Name: "Devin CLI", Kind: KindCLI,
		Site:     "https://docs.devin.ai",
		Commands: []string{"devin"},
		Locations: Paths{
			"windows": {npmWin + "devin.cmd"},
			"*":       unixBins("devin"),
		},
		Package: "agent-plugins",
		Steps:   [][]string{{"plugins", "install", "blaktron/agentry-plugin-agent-plugins", "-y"}},
	},
	{
		ID: "t3code", Name: "T3 Code", Kind: KindApp,
		Site: "https://t3.codes",
		Locations: Paths{
			"darwin": {
				"/Applications/T3 Code (Beta).app", "~/Applications/T3 Code (Beta).app",
				"/Applications/T3 Code.app", "~/Applications/T3 Code.app",
			},
			"windows": {"${LOCALAPPDATA}/Programs/T3 Code/T3 Code.exe"},
			"linux":   {"/opt/T3 Code"},
		},
		Uses: []string{"claude-code", "codex", "opencode", "cursor", "agy"},
		Note: "T3 Code has no plugins of its own: it runs Claude Code, Codex and the other agents, and their plugins come with them.",
	},
	{
		ID: "kiro", Name: "Kiro", Kind: KindEditor,
		Site:     "https://kiro.dev",
		Commands: []string{"kiro"},
		Locations: Paths{
			"darwin":  {"/Applications/Kiro.app", "~/Applications/Kiro.app"},
			"windows": {"${LOCALAPPDATA}/Programs/Kiro/Kiro.exe"},
			"linux":   {"/usr/share/kiro/kiro", "/opt/Kiro/kiro"},
		},
		Package:   "agent-plugins",
		Manual:    "Kiro adds the plugin as a power in its own window: open the Powers panel, choose Add Custom Power, then Import power from GitHub, and give it https://github.com/blaktron/agentry-plugin-agent-plugins.",
		ManualURL: "https://github.com/blaktron/agentry-plugin-agent-plugins#install",
	},
	{
		ID: "goose", Name: "goose", Kind: KindApp,
		Site: "https://block.github.io/goose/",
		// The goose command is not looked up on PATH: Homebrew's formula of
		// that name is an unrelated database tool.
		Locations: Paths{
			"darwin": {"/Applications/Goose.app", "~/Applications/Goose.app", "~/.local/bin/goose"},
			"*":      {"~/.local/bin/goose", "/usr/bin/Goose", "/usr/lib/goose/Goose"},
		},
		Note: "There is no Agentry plugin for goose yet: its plugins carry skills, not the server the Agentry plugin runs.",
	},
	{
		ID: "cursor", Name: "Cursor", Kind: KindEditor,
		Site:     "https://cursor.com/download",
		Commands: []string{"cursor"},
		Locations: Paths{
			"darwin":  {"/Applications/Cursor.app", "~/Applications/Cursor.app"},
			"windows": {"${LOCALAPPDATA}/Programs/cursor/Cursor.exe"},
			"linux":   {"/usr/share/cursor/cursor", "/opt/Cursor/cursor", "/opt/cursor/cursor"},
		},
		Package:   "cursor",
		Markers:   Paths{"*": {"~/.cursor/plugins/local/agentry/.cursor-plugin/plugin.json"}},
		Manual:    "Cursor adds plugins in its own window. Until Agentry is in the Cursor Marketplace, copy the plugin's repository into ~/.cursor/plugins/local/agentry and reload the window; its README has the steps.",
		ManualURL: "https://github.com/blaktron/agentry-plugin-cursor#install",
	},
}
