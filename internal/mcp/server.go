package mcp

import (
	"fmt"

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

// CreateMcpServer registers search/locate/expand/find_related/auth tools.
func CreateMcpServer(cache *IndexCache, benchmark bool) *MiruMcpServer {
	_ = benchmark // full benchmark mode omitted in Go port
	instructions := installer.MCPServerInstructions
	server := NewMiruMcpServer(MiruMcpServerInfo{Name: "miru", Version: version.MiruVersion()}, instructions)
	registerAuthTool(server)
	registerSearchTools(server, cache)
	return server
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
				return toolErrorText(err), nil
			}
			k := utils.ClampMCPTopK(IntArg(args, "top_k"))
			results, err := idx.Search(miruindex.SearchOptions{Query: query, TopK: k})
			if err != nil {
				return toolErrorText(err), nil
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
				return toolErrorText(err), nil
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
				return toolErrorText(err), nil
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
				return toolErrorText(err), nil
			}
			repoRoot := utils.LocalRepoRoot(repo)
			chunk := utils.ResolveChunk(idx.Chunks(), filePath, *anchorLinePtr, repoRoot)
			if chunk == nil {
				return ToolText(fmt.Sprintf("No chunk found at %s:%d. Make sure the file is indexed and the line number is within a known chunk.", filePath, *anchorLinePtr)), nil
			}
			k := utils.ClampMCPTopK(IntArg(args, "top_k"))
			results, err := idx.FindRelated(*chunk, k)
			if err != nil {
				return toolErrorText(err), nil
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
