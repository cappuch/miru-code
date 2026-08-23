package chunk

import (
	"os"
	"strings"
	"sync"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/bash"
	"github.com/smacker/go-tree-sitter/c"
	"github.com/smacker/go-tree-sitter/cpp"
	"github.com/smacker/go-tree-sitter/csharp"
	"github.com/smacker/go-tree-sitter/css"
	"github.com/smacker/go-tree-sitter/dockerfile"
	"github.com/smacker/go-tree-sitter/elixir"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/html"
	"github.com/smacker/go-tree-sitter/java"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/kotlin"
	"github.com/smacker/go-tree-sitter/lua"
	"github.com/smacker/go-tree-sitter/php"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/ruby"
	"github.com/smacker/go-tree-sitter/rust"
	"github.com/smacker/go-tree-sitter/scala"
	"github.com/smacker/go-tree-sitter/toml"
	"github.com/smacker/go-tree-sitter/typescript/tsx"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
	"github.com/smacker/go-tree-sitter/yaml"
)

const (
	astRecursionDepth = 500
	astMinChunkSize   = 50
)

func astChunkingEnabled() bool {
	return os.Getenv("MIRU_AST_CHUNKING") != "0"
}

var (
	langMu    sync.Mutex
	langCache = map[string]*sitter.Language{}
)

func languageForFile(filePath string, language *string) *sitter.Language {
	if language == nil || *language == "" {
		return nil
	}
	lang := *language
	key := lang
	lower := strings.ToLower(filePath)
	if lang == "typescript" && strings.HasSuffix(lower, ".tsx") {
		key = "tsx"
	}

	langMu.Lock()
	defer langMu.Unlock()
	if cached, ok := langCache[key]; ok {
		return cached
	}
	var l *sitter.Language
	switch key {
	case "bash":
		l = bash.GetLanguage()
	case "c":
		l = c.GetLanguage()
	case "cpp":
		l = cpp.GetLanguage()
	case "csharp":
		l = csharp.GetLanguage()
	case "css":
		l = css.GetLanguage()
	case "dockerfile":
		l = dockerfile.GetLanguage()
	case "elixir":
		l = elixir.GetLanguage()
	case "go":
		l = golang.GetLanguage()
	case "html":
		l = html.GetLanguage()
	case "java":
		l = java.GetLanguage()
	case "javascript":
		l = javascript.GetLanguage()
	case "kotlin":
		l = kotlin.GetLanguage()
	case "lua":
		l = lua.GetLanguage()
	case "php":
		l = php.GetLanguage()
	case "python":
		l = python.GetLanguage()
	case "ruby":
		l = ruby.GetLanguage()
	case "rust":
		l = rust.GetLanguage()
	case "scala":
		l = scala.GetLanguage()
	case "toml":
		l = toml.GetLanguage()
	case "typescript":
		l = typescript.GetLanguage()
	case "tsx":
		l = tsx.GetLanguage()
	case "yaml":
		l = yaml.GetLanguage()
	default:
		return nil
	}
	langCache[key] = l
	return l
}

func mergeNodeInner(node *sitter.Node, desiredLength, depth int) []ChunkBoundary {
	if node == nil {
		return nil
	}
	if node.ChildCount() == 0 {
		return []ChunkBoundary{{Start: int(node.StartByte()), End: int(node.EndByte())}}
	}
	length := int(node.EndByte() - node.StartByte())
	if depth > astRecursionDepth {
		return []ChunkBoundary{{Start: int(node.StartByte()), End: int(node.EndByte())}}
	}
	if length < astMinChunkSize {
		return []ChunkBoundary{{Start: int(node.StartByte()), End: int(node.EndByte())}}
	}

	groups := make([]ChunkBoundary, 0)
	childCount := int(node.ChildCount())
	index := 0
	for index < childCount {
		child := node.Child(index)
		if child == nil {
			break
		}
		start := int(child.StartByte())
		end := int(child.EndByte())
		groupLength := end - start
		index++

		if groupLength > desiredLength {
			groups = append(groups, mergeNodeInner(child, desiredLength, depth+1)...)
			continue
		}
		for index < childCount {
			next := node.Child(index)
			if next == nil {
				break
			}
			childLength := int(next.EndByte() - next.StartByte())
			if groupLength+childLength > desiredLength {
				break
			}
			end = int(next.EndByte())
			groupLength += childLength
			index++
		}
		groups = append(groups, ChunkBoundary{Start: start, End: end})
	}
	return groups
}

func mergeNode(node *sitter.Node, desiredLength int) []ChunkBoundary {
	raw := mergeNodeInner(node, desiredLength, 0)
	return MergeAdjacentChunks(raw, desiredLength)
}

// ChunkAst returns AST-aware byte boundaries, or nil to fall back (matches ast.ts cascade).
// Offsets are UTF-8 byte indices (tree-sitter + Go strings); TS converts to UTF-16 for JS.
func ChunkAst(source, filePath string, language *string, desiredLength int) []ChunkBoundary {
	if strings.TrimSpace(source) == "" || !astChunkingEnabled() {
		return nil
	}
	lang := languageForFile(filePath, language)
	if lang == nil {
		return nil
	}
	parser := sitter.NewParser()
	parser.SetLanguage(lang)
	tree := parser.Parse(nil, []byte(source))
	if tree == nil {
		return nil
	}
	defer tree.Close()
	root := tree.RootNode()
	if root == nil {
		return nil
	}
	return mergeNode(root, desiredLength)
}
