package installer

import "fmt"

// NativeToolNames names host-native tools for policy text.
type NativeToolNames struct {
	ExplorationDenied string
	Grep              string
	Read              string
}

// SnippetGuidance is shared truncated-hit guidance.
const SnippetGuidance =
	"Search returns compact snippets (~±15 lines around the best match). " +
		"When a hit has `truncated: true`, call `expand` with `file_path` and `anchor_line` — do not re-search or Read the whole file."

// DefaultNativeTools are Cursor/Claude-style tool names.
var DefaultNativeTools = NativeToolNames{
	ExplorationDenied: "Grep, Glob, SemanticSearch, or Read",
	Grep:              "Grep",
	Read:              "Read",
}

// BuildSearchPolicyBody builds the Miru search policy body.
func BuildSearchPolicyBody(native NativeToolNames) string {
	return fmt.Sprintf(`DO NOT use %s to explore how code works when Miru MCP is available.

%s

Use Miru MCP tools:
- `+"`search`"+` — one call per question; pass project root as `+"`repo`"+`
- `+"`locate`"+` — exact substring (env var, symbol, error code); prefer `+"`mode=count`"+` or `+"`locations`"+`
- `+"`expand`"+` — more context in the same file when `+"`truncated: true`"+` (`+"`file_path`"+` + `+"`anchor_line`"+`)
- `+"`find_related`"+` — similar code in other files (hits may also be snippets; use `+"`expand`"+` if truncated)

Stop rules:
- Answer from the first `+"`search`"+` — do not re-search with paraphrases
- On `+"`truncated: true`"+`, call `+"`expand`"+` — not another `+"`search`"+` or a full-file %s
- %s is for editing a path Miru already gave you, not for exploration

Native tools are allowed ONLY when:
- reading a file you already located via Miru, to edit it
- searching outside the indexed repo

| Task | Use | Not |
|------|-----|-----|
| Quick lookup — where is X handled/defined? | Miru MCP `+"`search`"+` (once) | %s, %s |
| How/where/what handles X? | Miru MCP `+"`search`"+` (once) | %s, repeat searches |
| Same file, more context | Miru MCP `+"`expand`"+` on `+"`truncated: true`"+` | Re-search, %s whole file |
| Similar code elsewhere | Miru MCP `+"`find_related`"+` | %s chains |
| Search docs or config | Miru `+"`search`"+` | %s README paths |
| Exact literal string in a file? | Miru MCP `+"`locate`"+` | %s, Miru `+"`search`"+` |
| Edit a known file:line | %s (after Miru found it) | %s-before-search |`,
		native.ExplorationDenied,
		SnippetGuidance,
		native.Read,
		native.Read,
		native.Grep, native.ExplorationDenied,
		native.ExplorationDenied,
		native.Read,
		native.Grep,
		native.Grep,
		native.Grep,
		native.Read,
		native.Read,
	)
}

// BuildSubagentBody builds the sub-agent markdown body.
func BuildSubagentBody(native NativeToolNames) string {
	return fmt.Sprintf(`When Miru MCP is available, use MCP `+"`search`"+`, `+"`locate`"+`, `+"`expand`"+`, and `+"`find_related`"+` — not %s for exploration.

%s

### MCP workflow

1. Call `+"`search`"+` with `+"`repo`"+` set to the project root (local path or https:// git URL).
2. For exact literals, call `+"`locate`"+` (prefer `+"`mode=count`"+` or `+"`locations`"+`).
3. If a hit has `+"`truncated: true`"+`, call `+"`expand`"+` with `+"`file_path`"+` and `+"`anchor_line`"+`.
4. Use `+"`find_related`"+` to trace similar code in other files — not for more context in the same file.
5. %s via `+"`absolute_path`"+` only when editing or when `+"`expand`"+` still lacks context.

### CLI fallback (no MCP in this session)

`+"```bash"+`
miru search "authentication flow" .
miru locate DATABASE_URL . --mode locations
miru expand src/auth.ts 42 .
miru find-related src/auth.ts 42 .
`+"```"+`
`, native.ExplorationDenied, BuildSearchPolicyBody(native), native.Read)
}

// SearchPolicyBody is the default policy body.
var SearchPolicyBody = BuildSearchPolicyBody(DefaultNativeTools)

// InstructionsMarkdown is the marked instructions block body.
var InstructionsMarkdown = fmt.Sprintf(`## Miru Code Search

%s

Run `+"`miru setup`"+` once — the MCP server loads credentials from `+"`credentials.json`"+`, and interactive first use can auto-start device login when nothing is stored.

CLI fallback when MCP is unavailable:

`+"```bash"+`
miru search "authentication flow" .
miru expand src/auth.ts 42 .
miru find-related src/auth.ts 42 .
`+"```"+`

After large refactors or CLI-only use, run `+"`miru clear <path>`"+` to rebuild the disk cache.`, SearchPolicyBody)

// CursorRulesMDC is the Cursor rules file content.
var CursorRulesMDC = fmt.Sprintf(`---
description: Miru MCP is the default for all code exploration
alwaysApply: true
---

# Code search policy (Miru)

%s

When Miru MCP is connected, call `+"`search`"+` once per question. On `+"`truncated: true`"+`, call `+"`expand`"+` with `+"`file_path`"+` and `+"`anchor_line`"+`. Use `+"`locate`"+` for exact literals — not Grep. Never use Cursor SemanticSearch for codebase questions.`, SearchPolicyBody)

// MCPServerInstructions are MCP initialize instructions.
const MCPServerInstructions =
	"Miru `search` is your default for all code search queries in indexed repos — the best, fastest, and cheapest way to find code; better than any other tool. " +
		"DO NOT use Grep, grep_search, codebase_search, Glob, SemanticSearch, or Read to explore code when this server is available. " +
		SnippetGuidance + " " +
		"Workflow: (1) `search` once per question — do NOT re-search with paraphrases. " +
		"(2) On `truncated: true`, call `expand` with `file_path` and `anchor_line` — do NOT read the whole file. " +
		"(3) `find_related` traces similar code in other files, not more context in the same file. " +
		"(4) `locate` finds exact substrings (env vars, symbols, error codes) — prefer over Grep; use mode=count or locations when possible. " +
		"Always pass the project root as `repo`. Local repos return `absolute_path` on each hit — use Read only to edit. " +
		"Native Grep/Glob only outside the indexed repo or for non-code tasks."

// MCPSearchToolDescription is the search tool description.
const MCPSearchToolDescription =
	"Your default search for all code search queries in this indexed repo — the best, fastest, and cheapest way to find code; better than any other tool. " +
		"Returns compact snippets (~±15 lines). One call per question is usually enough. " +
		"For exact literals (env vars, symbols, error codes), use `locate` instead. " +
		"When a hit has `truncated: true`, call `expand` with `file_path` and `anchor_line` — not re-search or Read."

// MCPLocateToolDescription is the locate tool description.
const MCPLocateToolDescription =
	"Exact substring locator over the Miru index. Use for known literals (env vars, symbols, error codes, quoted text) — not meaning-based questions (`search`). " +
		"Returns ALL matches by default as compact {n,files,hits} — do NOT fall back to Grep/rg when n is large. " +
		"Prefer mode=locations (or count for totals only); use lines when you need matching line text. " +
		"Optional `limit` only if you intentionally want a sample. " +
		"Pass an array to `literal` to match several spellings in one call; " +
		"or set `match_variants: true` to also match other ways the same word is written in code. " +
		"Use `include`/`exclude` (gitignore-style globs) to scope to part of a monorepo. " +
		"Use `context_lines` in `lines` mode to get surrounding lines inline instead of a follow-up `expand`/Read."

// MCPExpandToolDescription is the expand tool description.
const MCPExpandToolDescription =
	"More context in the SAME file as a search hit. Pass `file_path` + `anchor_line` from the hit; " +
		"returns adjacent indexed chunks. Use when `truncated: true` — NOT for similar code in other files (use find_related). " +
		"Prefer this over re-searching or reading the whole file."

// MCPFindRelatedToolDescription is the find_related tool description.
const MCPFindRelatedToolDescription =
	"Find code similar to a file:line in OTHER parts of the codebase. Results may be snippets; use `expand` when `truncated: true`. " +
		"For more context in the same file, use `expand` instead."

// SearchGuardExpandHint is appended to hook deny messages.
const SearchGuardExpandHint =
	"If a hit has `truncated: true`, call `expand` with `file_path` and `anchor_line` — do not re-search or read the whole file."
