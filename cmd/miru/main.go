package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/takara-ai/miru-code/internal/agents"
	"github.com/takara-ai/miru-code/internal/cache"
	"github.com/takara-ai/miru-code/internal/cliui"
	"github.com/takara-ai/miru-code/internal/credentials"
	"github.com/takara-ai/miru-code/internal/env"
	"github.com/takara-ai/miru-code/internal/help"
	"github.com/takara-ai/miru-code/internal/installer"
	"github.com/takara-ai/miru-code/internal/installer/hooks"
	"github.com/takara-ai/miru-code/internal/literal"
	"github.com/takara-ai/miru-code/internal/mcp"
	"github.com/takara-ai/miru-code/internal/miruindex"
	"github.com/takara-ai/miru-code/internal/setup"
	"github.com/takara-ai/miru-code/internal/types"
	"github.com/takara-ai/miru-code/internal/utils"
	"github.com/takara-ai/miru-code/internal/version"
)

var cliCommands = map[string]struct{}{
	"search": {}, "locate": {}, "expand": {}, "find-related": {},
	"init": {}, "install": {}, "uninstall": {}, "setup": {}, "clear": {},
	"benchmark": {}, "hook-guard": {}, "help": {}, "-h": {}, "--help": {},
	"-v": {}, "--version": {},
}

func main() {
	os.Args[0] = "miru"
	env.NormalizeTakaraAPIKeyEnv()
	_, _ = credentials.LoadStoredCredentials()

	argv := os.Args[1:]
	if len(argv) == 0 {
		runMCP(argv)
		return
	}
	first := argv[0]
	if first == "-v" || first == "--version" {
		fmt.Println(version.MiruVersion())
		return
	}
	if first == "hook-guard" {
		os.Exit(hooks.RunSearchGuardFromStdin(os.Stdin))
	}
	if _, ok := cliCommands[first]; ok {
		if err := runCLI(argv); err != nil {
			cliui.Fail(err.Error())
			os.Exit(1)
		}
		return
	}
	runMCP(argv)
}

func runMCP(argv []string) {
	var ref *string
	benchmark := false
	contentTokens := []string{}
	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		if arg == "--benchmark" {
			benchmark = true
			continue
		}
		if arg == "--ref" && i+1 < len(argv) {
			i++
			v := argv[i]
			ref = &v
			continue
		}
		if arg == "--content" {
			i++
			for i < len(argv) && !strings.HasPrefix(argv[i], "-") {
				contentTokens = append(contentTokens, argv[i])
				i++
			}
			i--
		}
	}
	_, _ = credentials.LoadStoredCredentials()
	if err := mcp.ServeMcp(mcp.ServeOptions{
		Ref:       ref,
		Content:   utils.ResolveContent(contentTokens),
		Benchmark: benchmark,
	}); err != nil {
		cliui.Fail(err.Error())
		os.Exit(1)
	}
}

func runCLI(argv []string) error {
	command := argv[0]
	rest := argv[1:]

	switch command {
	case "-h", "--help":
		help.PrintFullHelp()
		return nil
	case "-v", "--version":
		fmt.Println(version.MiruVersion())
		return nil
	case "help":
		if len(rest) == 0 {
			help.PrintMainHelp()
			return nil
		}
		help.PrintCommandHelp(rest[0])
		return nil
	case "hook-guard":
		os.Exit(hooks.RunSearchGuardFromStdin(os.Stdin))
	case "install", "uninstall":
		opts := installer.RunInstallerOptions{}
		mode := installer.ModeInstall
		if command == "uninstall" {
			mode = installer.ModeUninstall
		}
		filtered := make([]string, 0, len(rest))
		for i := 0; i < len(rest); i++ {
			switch rest[i] {
			case "--yes", "-y":
				opts.Yes = true
			case "--all":
				opts.AllAgents = true
			case "--agent":
				if i+1 < len(rest) {
					i++
					opts.AgentIDs = append(opts.AgentIDs, rest[i])
				}
			default:
				filtered = append(filtered, rest[i])
			}
		}
		_ = filtered
		return installer.RunInstaller(mode, opts)
	case "benchmark":
		return runBenchmark(rest)
	case "init":
		return runInit(rest)
	case "setup":
		return runSetup(rest)
	case "clear":
		path := utils.ResolveSearchPath(".")
		if len(rest) > 0 {
			path = utils.ResolveSearchPath(rest[0])
		}
		if err := cache.ClearCache(path); err != nil {
			return err
		}
		cliui.Success("Cleared cached index for " + path)
		return nil
	}

	jsonFlag, jsonRest := parseFlag(rest, "--json")
	content, contentRest := parseContent(jsonRest)
	topK, sizedRest := parseTopK(contentRest)

	switch command {
	case "search":
		if len(sizedRest) == 0 {
			help.PrintCommandHelp("search")
			os.Exit(1)
		}
		query := sizedRest[0]
		path := utils.ResolveSearchPath(".")
		if len(sizedRest) > 1 {
			path = utils.ResolveSearchPath(sizedRest[1])
		}
		return runSearch(path, query, topK, content, jsonFlag)
	case "locate":
		if len(sizedRest) == 0 {
			help.PrintCommandHelp("locate")
			os.Exit(1)
		}
		return runLocate(sizedRest, content, jsonFlag)
	case "expand":
		return runExpand(sizedRest, content, jsonFlag)
	case "find-related":
		return runFindRelated(sizedRest, content, topK, jsonFlag)
	default:
		cliui.Fail("Unknown command: " + command)
		help.PrintMainHelp()
		os.Exit(1)
	}
	return nil
}

func runSetup(rest []string) error {
	args, errCode := setup.ParseSetupCLIArgs(rest)
	switch errCode {
	case setup.ErrClearWithKey:
		cliui.Fail("miru setup --clear cannot be combined with --key, --device, or --sagemaker.")
		os.Exit(1)
	case setup.ErrSageMakerWithKey:
		cliui.Fail("miru setup --sagemaker cannot be combined with --key.")
		os.Exit(1)
	case setup.ErrDeviceWithKey:
		cliui.Fail("miru setup accepts either --device or --key TOKEN, not both.")
		os.Exit(1)
	case setup.ErrDeviceWithSageMaker:
		cliui.Fail("miru setup --device cannot be combined with --sagemaker.")
		os.Exit(1)
	}
	if args.Clear {
		return setup.RunClearCredentials()
	}
	result, err := setup.RunSetup(setup.RunSetupOptions{
		APIKey:       args.APIKey,
		Device:       args.Device,
		Force:        args.Force,
		SageMaker:    args.SageMaker,
		SageMakerARN: args.SageMakerARN,
		Profile:      args.Profile,
	})
	if err != nil {
		return err
	}
	if result.NewlySaved {
		if setup.CanPromptForCredentials() && args.APIKey == "" && !args.Device && !args.Force {
			if installer.PromptConfirm("Configure Miru in your coding agent now?", true) {
				return installer.RunInstaller(installer.ModeInstall, installer.RunInstallerOptions{})
			}
		}
		cliui.Hint("Run `miru install` to add Miru to your IDE.")
	}
	return nil
}

func runInit(rest []string) error {
	var agent string
	force := false
	for i := 0; i < len(rest); i++ {
		arg := rest[i]
		if arg == "--force" {
			force = true
			continue
		}
		if (arg == "--agent" || arg == "-a") && i+1 < len(rest) {
			i++
			agent = rest[i]
		}
	}
	if agent == "" {
		cliui.Fail("miru init requires --agent.")
		help.PrintCommandHelp("init")
		os.Exit(1)
	}
	if !agents.IsValidAgentID(agent) {
		cliui.Fail(help.FormatUnknownAgent(agent))
		os.Exit(1)
	}
	dest, err := agents.WriteAgentFile(agents.AgentID(agent), force)
	if err != nil {
		cliui.Fail(err.Error())
		cliui.Hint("Use --force to overwrite an existing file.")
		os.Exit(1)
	}
	cliui.Success("Wrote sub-agent: " + dest)
	return nil
}

func runBenchmark(rest []string) error {
	if len(rest) == 0 || rest[0] == "-h" || rest[0] == "--help" {
		help.PrintCommandHelp("benchmark")
		return nil
	}
	action := rest[0]
	switch action {
	case "status", "on", "off", "clear":
		cliui.Info("Benchmark mode toggling is simplified in the Go port.")
		cliui.Hint("Pass --benchmark when launching MCP, or reinstall agent configs with that flag preserved.")
		return nil
	default:
		cliui.Fail(fmt.Sprintf("Unknown benchmark action %q. Use on, off, status, or clear.", action))
		help.PrintCommandHelp("benchmark")
		os.Exit(1)
	}
	return nil
}

func runSearch(path, query string, topK int, content []types.ContentType, jsonFlag bool) error {
	if err := setup.EnsureCredentials(true); err != nil {
		return err
	}
	cliui.Info("Indexing and searching…")
	idx, err := miruindex.FromSource(path, content, nil, nil)
	if err != nil {
		return err
	}
	_ = idx.SaveToCache(path, false)
	results, err := idx.Search(miruindex.SearchOptions{Query: query, TopK: topK})
	if err != nil {
		return err
	}
	emitSearchOutput(query, results, jsonFlag, "No results found.")
	return nil
}

func runExpand(sizedRest []string, content []types.ContentType, jsonFlag bool) error {
	if len(sizedRest) < 2 {
		help.PrintCommandHelp("expand")
		os.Exit(1)
	}
	filePath := sizedRest[0]
	line, err := strconv.Atoi(sizedRest[1])
	if err != nil {
		return fmt.Errorf("invalid line number")
	}
	path := utils.ResolveSearchPath(".")
	before := utils.DefaultExpandBefore
	after := utils.DefaultExpandAfter
	for i := 2; i < len(sizedRest); i++ {
		arg := sizedRest[i]
		if arg == "--before" && i+1 < len(sizedRest) {
			i++
			before, _ = strconv.Atoi(sizedRest[i])
			continue
		}
		if arg == "--after" && i+1 < len(sizedRest) {
			i++
			after, _ = strconv.Atoi(sizedRest[i])
			continue
		}
		if !strings.HasPrefix(arg, "-") {
			path = utils.ResolveSearchPath(arg)
		}
	}
	if err := setup.EnsureCredentials(true); err != nil {
		return err
	}
	cliui.Info("Expanding chunks…")
	idx, err := miruindex.FromSource(path, content, nil, nil)
	if err != nil {
		return err
	}
	repoRoot := utils.LocalRepoRoot(path)
	anchor, expanded := utils.ExpandChunksAtLine(idx.Chunks(), filePath, line, repoRoot, before, after)
	_ = idx.SaveToCache(path, false)
	if anchor == nil {
		cliui.Fail(fmt.Sprintf("No chunk found at %s:%d.", filePath, line))
		os.Exit(1)
	}
	payload := utils.FormatExpandResults(filePath, line, anchor, expanded, &utils.FormatExpandOptions{
		RepoRoot: repoRoot, Before: &before, After: &after,
	})
	if cliui.PrefersJSONOutput(jsonFlag) {
		enc := json.NewEncoder(os.Stdout)
		return enc.Encode(payload)
	}
	for _, chunk := range payload.Chunks {
		fmt.Printf("\n%v\n", chunk["location"])
		fmt.Printf("%v\n", chunk["content"])
	}
	fmt.Println()
	return nil
}

func runFindRelated(sizedRest []string, content []types.ContentType, topK int, jsonFlag bool) error {
	if len(sizedRest) < 2 {
		help.PrintCommandHelp("find-related")
		os.Exit(1)
	}
	filePath := sizedRest[0]
	line, err := strconv.Atoi(sizedRest[1])
	if err != nil {
		return fmt.Errorf("invalid line number")
	}
	path := utils.ResolveSearchPath(".")
	if len(sizedRest) > 2 {
		path = utils.ResolveSearchPath(sizedRest[2])
	}
	if err := setup.EnsureCredentials(true); err != nil {
		return err
	}
	cliui.Info("Finding related chunks…")
	idx, err := miruindex.FromSource(path, content, nil, nil)
	if err != nil {
		return err
	}
	chunk := utils.ResolveChunk(idx.Chunks(), filePath, line, utils.LocalRepoRoot(path))
	if chunk == nil {
		return fmt.Errorf("No chunk found at %s:%d.", filePath, line)
	}
	results, err := idx.FindRelated(*chunk, topK)
	if err != nil {
		return err
	}
	_ = idx.SaveToCache(path, false)
	emitSearchOutput(cliui.FormatRelatedHeader(filePath, line), results, jsonFlag, fmt.Sprintf("No related chunks found for %s:%d.", filePath, line))
	return nil
}

func runLocate(sizedRest []string, content []types.ContentType, jsonFlag bool) error {
	lit := sizedRest[0]
	opts := literal.LocateOptions{}
	var include, exclude []string
	pathArgs := []string{}
	for i := 1; i < len(sizedRest); i++ {
		arg := sizedRest[i]
		switch {
		case arg == "--mode" && i+1 < len(sizedRest):
			i++
			opts.Mode = literal.Mode(sizedRest[i])
		case arg == "--limit" && i+1 < len(sizedRest):
			i++
			n, _ := strconv.Atoi(sizedRest[i])
			opts.Limit = &n
		case arg == "--ignore-case":
			opts.IgnoreCase = true
		case arg == "--match-variants":
			opts.MatchVariants = true
		case arg == "--include" && i+1 < len(sizedRest):
			i++
			include = append(include, sizedRest[i])
		case arg == "--exclude" && i+1 < len(sizedRest):
			i++
			exclude = append(exclude, sizedRest[i])
		case arg == "--context" && i+1 < len(sizedRest):
			i++
			n, _ := strconv.Atoi(sizedRest[i])
			opts.ContextLines = &n
		default:
			if !strings.HasPrefix(arg, "-") {
				pathArgs = append(pathArgs, arg)
			}
		}
	}
	opts.Include = include
	opts.Exclude = exclude
	path := utils.ResolveSearchPath(".")
	if len(pathArgs) > 0 {
		path = utils.ResolveSearchPath(pathArgs[0])
	}
	if err := setup.EnsureCredentials(true); err != nil {
		return err
	}
	cliui.Info("Locating literal…")
	idx, err := miruindex.FromSource(path, content, nil, nil)
	if err != nil {
		return err
	}
	_ = idx.SaveToCache(path, false)
	result := idx.LocateLiteral(lit, opts)
	payload := literal.FormatLocate(result)
	if cliui.PrefersJSONOutput(jsonFlag) {
		enc := json.NewEncoder(os.Stdout)
		return enc.Encode(payload)
	}
	mode := opts.Mode
	if mode == "" {
		mode = literal.DefaultMode
	}
	fmt.Printf("literal=%s  n=%v  files=%v  mode=%s\n", lit, payload["n"], payload["files"], mode)
	if hits, ok := payload["hits"].([]map[string]any); ok {
		for _, hit := range hits {
			if ctx, ok := hit["ctx"].([]string); ok {
				fmt.Printf("  %v:%v:\n", hit["f"], hit["l"])
				start := 0
				if v, ok := hit["ctx_l"].(*int); ok && v != nil {
					start = *v
				} else if v, ok := hit["ctx_l"].(int); ok {
					start = v
				}
				for i, line := range ctx {
					fmt.Printf("    %d: %s\n", start+i, line)
				}
			} else if t, ok := hit["t"]; ok {
				fmt.Printf("  %v:%v: %v\n", hit["f"], hit["l"], t)
			} else {
				fmt.Printf("  %v:%v\n", hit["f"], hit["l"])
			}
		}
	}
	fmt.Println()
	return nil
}

func emitSearchOutput(query string, results []types.SearchResult, jsonFlag bool, emptyMessage string) {
	if len(results) == 0 {
		if cliui.PrefersJSONOutput(jsonFlag) {
			_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"error": emptyMessage})
			return
		}
		fmt.Print(cliui.FormatSearchErrorPretty(emptyMessage))
		return
	}
	if cliui.PrefersJSONOutput(jsonFlag) {
		_ = json.NewEncoder(os.Stdout).Encode(utils.FormatResults(query, results, nil))
		return
	}
	fmt.Print(cliui.FormatSearchResultsPretty(query, results))
}

func parseFlag(argv []string, flag string) (bool, []string) {
	rest := make([]string, 0, len(argv))
	present := false
	for _, arg := range argv {
		if arg == flag {
			present = true
			continue
		}
		rest = append(rest, arg)
	}
	return present, rest
}

func parseContent(argv []string) ([]types.ContentType, []string) {
	rest := make([]string, 0, len(argv))
	content := []string{}
	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		if arg == "--content" {
			i++
			for i < len(argv) && !strings.HasPrefix(argv[i], "-") {
				content = append(content, argv[i])
				i++
			}
			i--
			continue
		}
		rest = append(rest, arg)
	}
	return utils.ResolveContent(content), rest
}

func parseTopK(argv []string) (int, []string) {
	rest := make([]string, 0, len(argv))
	topK := 5
	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		if (arg == "-k" || arg == "--top-k") && i+1 < len(argv) {
			i++
			if n, err := strconv.Atoi(argv[i]); err == nil && n >= 1 {
				topK = n
			}
			continue
		}
		rest = append(rest, arg)
	}
	return topK, rest
}
