package mcp

import (
	"fmt"

	"github.com/takara-ai/miru-code/internal/auth"
	"github.com/takara-ai/miru-code/internal/credentials"
	"github.com/takara-ai/miru-code/internal/installer"
	"github.com/takara-ai/miru-code/internal/literal"
	"github.com/takara-ai/miru-code/internal/miruindex"
	"github.com/takara-ai/miru-code/internal/utils"
	"github.com/takara-ai/miru-code/internal/version"
)

const repoDescription =
	"https:// or http:// git URL (e.g. https://github.com/org/repo) or local directory path to index and search. " +
		"Pass the project root for local workspaces. " +
		"The index is built on the first tool call and cached for the session."

const authToolDescription =
	"Sign in with Takara credentials via device-code login — no terminal required. " +
		"Only call this in direct response to a tool error mentioning missing/expired " +
		"credentials — never speculatively, since it starts a real sign-in prompt for the " +
		"user. Call with no arguments (or action \"start\") to begin: it returns a URL and a " +
		"short code. Show both to the user and ask them to open the link and approve. Once " +
		"they confirm, call again with action \"check\" to complete sign-in."

type pendingDeviceAuth struct {
	start  auth.DeviceAuthorizationStart
	config auth.DeviceAuthConfig
}

// CreateMcpServer registers search/locate/expand/find_related/auth tools.
func CreateMcpServer(cache *IndexCache, benchmark bool) *MiruMcpServer {
	_ = benchmark // full benchmark mode omitted in Go port
	instructions := installer.MCPServerInstructions
	server := NewMiruMcpServer(MiruMcpServerInfo{Name: "miru", Version: version.MiruVersion()}, instructions)
	registerAuthTool(server)
	registerSearchTools(server, cache)
	return server
}

func registerAuthTool(server *MiruMcpServer) {
	var pending *pendingDeviceAuth
	server.RegisterTool("auth", ToolSchema{
		Description: authToolDescription,
		InputSchema: ObjectSchema(map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"start", "check"},
				"description": `"start" begins a device-code login (default); "check" completes it.`,
			},
		}, nil),
		Handler: func(args map[string]any) (ToolResult, error) {
			action := StringArg(args, "action")
			if action == "check" {
				return authCheck(&pending)
			}
			return authStart(&pending)
		},
	})
}

func authStart(pending **pendingDeviceAuth) (ToolResult, error) {
	if *pending != nil {
		start := (*pending).start
		link := start.VerificationURI
		if start.VerificationURIComplete != "" {
			link = start.VerificationURIComplete
		}
		return ToolText(fmt.Sprintf(
			"A device login is already pending. Open %s and enter code %s if not already filled in, then call `auth` again with action \"check\" once approved.",
			link, start.UserCode,
		)), nil
	}
	config := auth.ResolveDeviceAuthConfig()
	start, err := auth.StartDeviceAuthorization(&config, nil)
	if err != nil {
		return ToolText(err.Error()), nil
	}
	*pending = &pendingDeviceAuth{start: start, config: config}
	link := start.VerificationURI
	msg := fmt.Sprintf("Open %s and approve the request", link)
	if start.VerificationURIComplete != "" {
		link = start.VerificationURIComplete
		msg = fmt.Sprintf("Open %s and approve the request.", link)
	} else {
		msg += fmt.Sprintf(" (enter code %s if prompted).", start.UserCode)
	}
	msg += fmt.Sprintf(" Code: %s. Once the user confirms they've approved it, call `auth` again with action \"check\" to finish signing in.", start.UserCode)
	return ToolText(msg), nil
}

func authCheck(pending **pendingDeviceAuth) (ToolResult, error) {
	if *pending == nil {
		return ToolText("No device login is pending. Call `auth` with action \"start\" first."), nil
	}
	result, err := auth.CheckDeviceAuthorizationOnce((*pending).start, &(*pending).config, nil)
	if err != nil {
		return ToolText(err.Error()), nil
	}
	switch result.Status {
	case "success":
		*pending = nil
		_, _ = credentials.SaveDeviceCode(auth.SaveDeviceCodeInput{
			AccessToken:  result.Tokens.AccessToken,
			RefreshToken: result.Tokens.RefreshToken,
			ExpiresAt:    result.Tokens.ExpiresAt,
			TokenType:    result.Tokens.TokenType,
			Scope:        result.Tokens.Scope,
		})
		credentials.SetStoredCredentialsEnvToken(result.Tokens.AccessToken)
		return ToolText("Signed in successfully. Miru tools are now ready to use."), nil
	case "pending":
		return ToolText("Still waiting for approval. Ask the user to confirm they clicked and approved, then call `auth` again with action \"check\"."), nil
	case "slow_down":
		return ToolText("Checking too soon — wait a bit before calling `auth` again with action \"check\"."), nil
	case "denied":
		*pending = nil
		return ToolText("Sign-in was denied. Call `auth` with action \"start\" to try again."), nil
	case "expired":
		*pending = nil
		return ToolText("The device code expired before it was approved. Call `auth` with action \"start\" to try again."), nil
	default:
		return ToolText("Unexpected auth status."), nil
	}
}

func registerSearchTools(server *MiruMcpServer, cache *IndexCache) {
	server.RegisterTool("search", ToolSchema{
		Description: installer.MCPSearchToolDescription + " Indexes `repo` on first call; later calls reuse the session cache.",
		InputSchema: ObjectSchema(map[string]any{
			"query": Prop("string", "Natural language or code query — your default for all code search in this repo."),
			"repo":  Prop("string", repoDescription),
			"top_k": map[string]any{
				"type":        "integer",
				"minimum":     1,
				"maximum":     utils.MaxMCPTopK,
				"description": fmt.Sprintf("Number of results (default %d, max %d).", utils.DefaultMCPTopK, utils.MaxMCPTopK),
			},
			"dedupe_by_file": map[string]any{
				"type":        "boolean",
				"description": "Keep only the best hit per file (default true).",
			},
		}, []string{"query", "repo"}),
		Handler: func(args map[string]any) (ToolResult, error) {
			query := StringArg(args, "query")
			repo := StringArg(args, "repo")
			idx, err := GetIndexForRepo(repo, cache, nil)
			if err != nil {
				return ToolText(err.Error()), nil
			}
			k := utils.ClampMCPTopK(IntArg(args, "top_k"))
			results, err := idx.Search(miruindex.SearchOptions{Query: query, TopK: k})
			if err != nil {
				return ToolText(err.Error()), nil
			}
			dedupe := BoolArg(args, "dedupe_by_file")
			if dedupe == nil || *dedupe {
				results = utils.DedupeResultsByFile(results)
			}
			if len(results) == 0 {
				return ToolText("No results found."), nil
			}
			repoRoot := utils.LocalRepoRoot(repo)
			snippet := true
			payload := utils.FormatResults(query, results, &utils.FormatResultsOptions{RepoRoot: repoRoot, Snippet: &snippet})
			return ToolText(FormatResultsText(payload)), nil
		},
	})

	server.RegisterTool("locate", ToolSchema{
		Description: installer.MCPLocateToolDescription,
		InputSchema: ObjectSchema(map[string]any{
			"literal": map[string]any{
				"description": "Exact substring to find (env var, symbol, error code, quoted text). Pass an array to OR-match several substrings.",
			},
			"repo": Prop("string", repoDescription),
			"mode": map[string]any{
				"type":        "string",
				"enum":        []string{"count", "locations", "lines"},
				"description": "count=totals only; locations=path:line; lines=path:line+text (default).",
			},
			"limit": map[string]any{
				"type":        "integer",
				"minimum":     1,
				"description": "Optional cap on returned hits.",
			},
			"ignore_case":    map[string]any{"type": "boolean", "description": "Case-insensitive match (default false)."},
			"match_variants": map[string]any{"type": "boolean", "description": "Also match camelCase/snake_case/kebab-case/CONSTANT_CASE."},
			"include":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Gitignore-style include globs."},
			"exclude":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Gitignore-style exclude globs."},
			"context_lines":  map[string]any{"type": "integer", "minimum": 0, "description": "Lines of context around each match (mode=lines)."},
		}, []string{"literal", "repo"}),
		Handler: func(args map[string]any) (ToolResult, error) {
			repo := StringArg(args, "repo")
			idx, err := GetIndexForRepo(repo, cache, nil)
			if err != nil {
				return ToolText(err.Error()), nil
			}
			var lit any
			if raw, ok := args["literal"].([]any); ok {
				lits := make([]string, 0, len(raw))
				for _, item := range raw {
					if s, ok := item.(string); ok {
						lits = append(lits, s)
					}
				}
				lit = lits
			} else {
				lit = StringArg(args, "literal")
			}
			opts := literal.LocateOptions{
				IgnoreCase:    boolOr(args, "ignore_case", false),
				MatchVariants: boolOr(args, "match_variants", false),
				Include:       StringSliceArg(args, "include"),
				Exclude:       StringSliceArg(args, "exclude"),
			}
			if mode := StringArg(args, "mode"); mode != "" {
				opts.Mode = literal.Mode(mode)
			}
			opts.Limit = IntArg(args, "limit")
			opts.ContextLines = IntArg(args, "context_lines")
			result := idx.LocateLiteral(lit, opts)
			payload := literal.FormatLocate(result)
			return ToolText(FormatLiteralLocateText(payload)), nil
		},
	})

	server.RegisterTool("expand", ToolSchema{
		Description: installer.MCPExpandToolDescription,
		InputSchema: ObjectSchema(map[string]any{
			"file_path":   Prop("string", "Path from a search hit (`file_path` or `absolute_path` for local repos)."),
			"anchor_line": map[string]any{"type": "integer", "description": "Line from the search hit (`anchor_line` when truncated, else `start_line`)."},
			"repo":        Prop("string", repoDescription),
			"before":      map[string]any{"type": "integer", "minimum": 0, "description": fmt.Sprintf("Extra chunks before the anchor (default %d).", utils.DefaultExpandBefore)},
			"after":       map[string]any{"type": "integer", "minimum": 0, "description": fmt.Sprintf("Extra chunks after the anchor (default %d).", utils.DefaultExpandAfter)},
		}, []string{"file_path", "anchor_line", "repo"}),
		Handler: func(args map[string]any) (ToolResult, error) {
			filePath := StringArg(args, "file_path")
			anchorLinePtr := IntArg(args, "anchor_line")
			if filePath == "" || anchorLinePtr == nil {
				return ToolText("file_path and anchor_line are required."), nil
			}
			repo := StringArg(args, "repo")
			idx, err := GetIndexForRepo(repo, cache, nil)
			if err != nil {
				return ToolText(err.Error()), nil
			}
			before := utils.DefaultExpandBefore
			after := utils.DefaultExpandAfter
			if v := IntArg(args, "before"); v != nil {
				before = *v
			}
			if v := IntArg(args, "after"); v != nil {
				after = *v
			}
			repoRoot := utils.LocalRepoRoot(repo)
			anchor, expanded := utils.ExpandChunksAtLine(idx.Chunks(), filePath, *anchorLinePtr, repoRoot, before, after)
			if anchor == nil {
				return ToolText(fmt.Sprintf("No chunk found at %s:%d. Make sure the file is indexed and the line number is within a known chunk.", filePath, *anchorLinePtr)), nil
			}
			payload := utils.FormatExpandResults(filePath, *anchorLinePtr, anchor, expanded, &utils.FormatExpandOptions{
				RepoRoot: repoRoot,
				Before:   &before,
				After:    &after,
			})
			return ToolText(FormatExpandResultsText(payload)), nil
		},
	})

	server.RegisterTool("find_related", ToolSchema{
		Description: installer.MCPFindRelatedToolDescription,
		InputSchema: ObjectSchema(map[string]any{
			"file_path":   Prop("string", "Path from a search hit."),
			"anchor_line": map[string]any{"type": "integer", "description": "Line from the search hit."},
			"repo":        Prop("string", repoDescription),
			"top_k": map[string]any{
				"type":        "integer",
				"minimum":     1,
				"maximum":     utils.MaxMCPTopK,
				"description": fmt.Sprintf("Number of similar chunks (default %d, max %d).", utils.DefaultMCPTopK, utils.MaxMCPTopK),
			},
		}, []string{"file_path", "anchor_line", "repo"}),
		Handler: func(args map[string]any) (ToolResult, error) {
			filePath := StringArg(args, "file_path")
			anchorLinePtr := IntArg(args, "anchor_line")
			if filePath == "" || anchorLinePtr == nil {
				return ToolText("file_path and anchor_line are required."), nil
			}
			repo := StringArg(args, "repo")
			idx, err := GetIndexForRepo(repo, cache, nil)
			if err != nil {
				return ToolText(err.Error()), nil
			}
			repoRoot := utils.LocalRepoRoot(repo)
			chunk := utils.ResolveChunk(idx.Chunks(), filePath, *anchorLinePtr, repoRoot)
			if chunk == nil {
				return ToolText(fmt.Sprintf("No chunk found at %s:%d. Make sure the file is indexed and the line number is within a known chunk.", filePath, *anchorLinePtr)), nil
			}
			k := utils.ClampMCPTopK(IntArg(args, "top_k"))
			results, err := idx.FindRelated(*chunk, k)
			if err != nil {
				return ToolText(err.Error()), nil
			}
			if len(results) == 0 {
				return ToolText(fmt.Sprintf("No related chunks found for %s:%d.", filePath, *anchorLinePtr)), nil
			}
			snippet := true
			payload := utils.FormatResults(fmt.Sprintf("Chunks related to %s:%d", filePath, *anchorLinePtr), results, &utils.FormatResultsOptions{
				RepoRoot: repoRoot,
				Snippet:  &snippet,
			})
			return ToolText(FormatResultsText(payload)), nil
		},
	})
}

func boolOr(args map[string]any, key string, def bool) bool {
	if v := BoolArg(args, key); v != nil {
		return *v
	}
	return def
}
