package installer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// StripJSONComments removes // and /* */ comments outside strings.
func StripJSONComments(text string) string {
	var out strings.Builder
	inString := false
	stringQuote := byte('"')
	for i := 0; i < len(text); i++ {
		ch := text[i]
		var next byte
		if i+1 < len(text) {
			next = text[i+1]
		}
		if inString {
			out.WriteByte(ch)
			if ch == '\\' && i+1 < len(text) {
				out.WriteByte(text[i+1])
				i++
				continue
			}
			if ch == stringQuote {
				inString = false
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			inString = true
			stringQuote = ch
			out.WriteByte(ch)
			continue
		}
		if ch == '/' && next == '/' {
			for i < len(text) && text[i] != '\n' {
				i++
			}
			if i < len(text) {
				i--
			}
			continue
		}
		if ch == '/' && next == '*' {
			i += 2
			for i < len(text) && !(text[i] == '*' && i+1 < len(text) && text[i+1] == '/') {
				i++
			}
			i++ // skip '/'
			continue
		}
		out.WriteByte(ch)
	}
	return out.String()
}

func parseJSONObject(text string) (map[string]any, bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return map[string]any{}, true
	}
	var parsed any
	if err := json.Unmarshal([]byte(StripJSONComments(trimmed)), &parsed); err != nil {
		return nil, false
	}
	m, ok := parsed.(map[string]any)
	return m, ok
}

func ensureTrailingNewline(text string) string {
	if strings.HasSuffix(text, "\n") {
		return text
	}
	return text + "\n"
}

func withMergedMember(root map[string]any, sectionKey, memberKey string, value map[string]any) map[string]any {
	next := map[string]any{}
	for k, v := range root {
		next[k] = v
	}
	section := map[string]any{}
	if existing, ok := next[sectionKey].(map[string]any); ok {
		for k, v := range existing {
			section[k] = v
		}
	}
	section[memberKey] = value
	next[sectionKey] = section
	return next
}

func withRemovedMember(root map[string]any, sectionKey, memberKey string) map[string]any {
	next := map[string]any{}
	for k, v := range root {
		next[k] = v
	}
	section, ok := next[sectionKey].(map[string]any)
	if !ok {
		return next
	}
	sec := map[string]any{}
	for k, v := range section {
		if k != memberKey {
			sec[k] = v
		}
	}
	if len(sec) == 0 {
		delete(next, sectionKey)
	} else {
		next[sectionKey] = sec
	}
	return next
}

// MergeJSONMember upserts section.memberKey = value in a JSON config file.
func MergeJSONMember(path, sectionKey, memberKey string, value map[string]any) (InstallAction, error) {
	existed := false
	text := ""
	if data, err := os.ReadFile(path); err == nil {
		existed = true
		text = string(data)
	}
	parsed, ok := parseJSONObject(text)
	if !ok {
		return ActionError, nil
	}
	section, _ := parsed[sectionKey].(map[string]any)
	if section == nil {
		section = map[string]any{}
	}
	if existing, ok := section[memberKey].(map[string]any); ok {
		a, _ := json.Marshal(existing)
		b, _ := json.Marshal(value)
		if string(a) == string(b) {
			return ActionUnchanged, nil
		}
	}
	next := withMergedMember(parsed, sectionKey, memberKey, value)
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return ActionError, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return ActionError, err
	}
	if err := os.WriteFile(path, []byte(ensureTrailingNewline(string(data))), 0o644); err != nil {
		return ActionError, err
	}
	if existed {
		return ActionUpdated, nil
	}
	return ActionCreated, nil
}

// RemoveJSONMember removes section.memberKey from a JSON config file.
func RemoveJSONMember(path, sectionKey, memberKey string) (InstallAction, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ActionNotFound, nil
		}
		return ActionError, err
	}
	parsed, ok := parseJSONObject(string(data))
	if !ok {
		return ActionError, nil
	}
	section, _ := parsed[sectionKey].(map[string]any)
	if section == nil {
		return ActionNotFound, nil
	}
	if _, ok := section[memberKey]; !ok {
		return ActionNotFound, nil
	}
	next := withRemovedMember(parsed, sectionKey, memberKey)
	if len(next) == 0 {
		_ = os.Remove(path)
		return ActionRemoved, nil
	}
	out, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return ActionError, err
	}
	if err := os.WriteFile(path, []byte(ensureTrailingNewline(string(out))), 0o644); err != nil {
		return ActionError, err
	}
	return ActionRemoved, nil
}

// ReplaceOrAppendMarked writes/updates a <!-- miru:start --> block.
func ReplaceOrAppendMarked(path, content string) (InstallAction, error) {
	existed := false
	existing := ""
	if data, err := os.ReadFile(path); err == nil {
		existed = true
		existing = string(data)
	}
	startIdx := strings.Index(existing, MiruStart)
	endIdx := strings.Index(existing, MiruEnd)
	if startIdx != -1 && endIdx != -1 && endIdx > startIdx {
		before := existing[:startIdx]
		after := existing[endIdx+len(MiruEnd):]
		after = strings.TrimLeft(after, "\n")
		updated := before + strings.TrimSpace(content) + "\n" + after
		if updated == existing {
			return ActionUnchanged, nil
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return ActionError, err
		}
		if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
			return ActionError, err
		}
		return ActionUpdated, nil
	}
	separator := ""
	if existing != "" {
		if strings.HasSuffix(existing, "\n\n") {
			separator = ""
		} else if strings.HasSuffix(existing, "\n") {
			separator = "\n"
		} else {
			separator = "\n\n"
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return ActionError, err
	}
	if err := os.WriteFile(path, []byte(existing+separator+content), 0o644); err != nil {
		return ActionError, err
	}
	if existed {
		return ActionUpdated, nil
	}
	return ActionCreated, nil
}

// RemoveMarked removes a <!-- miru:start --> block.
func RemoveMarked(path string) (InstallAction, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ActionNotFound, nil
		}
		return ActionError, err
	}
	existing := string(data)
	startIdx := strings.Index(existing, MiruStart)
	endIdx := strings.Index(existing, MiruEnd)
	if startIdx == -1 || endIdx == -1 || endIdx <= startIdx {
		return ActionNotFound, nil
	}
	before := strings.TrimRight(existing[:startIdx], "\n")
	after := strings.TrimLeft(existing[endIdx+len(MiruEnd):], "\n")
	parts := []string{}
	if before != "" {
		parts = append(parts, before)
	}
	if after != "" {
		parts = append(parts, after)
	}
	updated := strings.Join(parts, "\n")
	if strings.TrimSpace(updated) == "" {
		_ = os.Remove(path)
		return ActionRemoved, nil
	}
	if err := os.WriteFile(path, []byte(updated+"\n"), 0o644); err != nil {
		return ActionError, err
	}
	return ActionRemoved, nil
}

const codexMCPHeader = "[mcp_servers.miru]"

func stripTomlSection(text, header string) string {
	prefix := strings.TrimSpace(header)
	prefix = strings.TrimPrefix(prefix, "[")
	prefix = strings.TrimSuffix(prefix, "]")
	lines := strings.Split(text, "\n")
	var result []string
	skipping := false
	for _, line := range lines {
		tableKey := strings.TrimSpace(strings.Split(line, "#")[0])
		if strings.HasPrefix(tableKey, "[") && strings.HasSuffix(tableKey, "]") {
			tableName := tableKey[1 : len(tableKey)-1]
			if tableName == prefix || strings.HasPrefix(tableName, prefix+".") {
				skipping = true
				continue
			}
			skipping = false
		}
		if !skipping {
			result = append(result, line)
		}
	}
	return strings.Join(result, "\n")
}

// MergeTomlBlock installs the Codex miru MCP block.
func MergeTomlBlock(path string) (InstallAction, error) {
	existed := false
	existing := ""
	if data, err := os.ReadFile(path); err == nil {
		existed = true
		existing = string(data)
	}
	defaultBlock := strings.TrimSpace(CodexTOMLBlock(false))
	benchmarkBlock := strings.TrimSpace(CodexTOMLBlock(true))
	if strings.Contains(existing, defaultBlock) || strings.Contains(existing, benchmarkBlock) {
		return ActionUnchanged, nil
	}
	preserveBenchmark := strings.Contains(existing, codexMCPHeader) &&
		(strings.Contains(existing, `"--benchmark"`) || strings.Contains(existing, `'--benchmark'`))
	block := CodexTOMLBlock(preserveBenchmark)
	base := strings.TrimRight(stripTomlSection(existing, codexMCPHeader), "\n")
	var next string
	if base != "" {
		next = base + "\n\n" + block
	} else {
		next = block
	}
	if !strings.HasSuffix(next, "\n") {
		next += "\n"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return ActionError, err
	}
	if err := os.WriteFile(path, []byte(next), 0o644); err != nil {
		return ActionError, err
	}
	if existed {
		return ActionUpdated, nil
	}
	return ActionCreated, nil
}

// RemoveTomlBlock removes the Codex miru MCP block.
func RemoveTomlBlock(path string) (InstallAction, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ActionNotFound, nil
		}
		return ActionError, err
	}
	existing := string(data)
	if !strings.Contains(existing, codexMCPHeader) {
		return ActionNotFound, nil
	}
	remaining := strings.TrimSpace(stripTomlSection(existing, codexMCPHeader))
	if remaining == "" {
		_ = os.Remove(path)
		return ActionRemoved, nil
	}
	if err := os.WriteFile(path, []byte(remaining+"\n"), 0o644); err != nil {
		return ActionError, err
	}
	return ActionRemoved, nil
}

// CodexSkillsFeatureEnabled reports whether [features] skills = true is set in Codex config.toml.
func CodexSkillsFeatureEnabled(text string) bool {
	lines := strings.Split(text, "\n")
	inFeatures := false
	for _, line := range lines {
		tableKey := strings.TrimSpace(strings.Split(line, "#")[0])
		if strings.HasPrefix(tableKey, "[") && strings.HasSuffix(tableKey, "]") {
			inFeatures = tableKey == "[features]"
			continue
		}
		if !inFeatures {
			continue
		}
		parts := strings.Split(line, "#")
		trimmed := strings.TrimSpace(parts[0])
		if len(trimmed) >= 5 && strings.HasPrefix(strings.ToLower(trimmed), "skills") {
			if strings.Contains(strings.ToLower(trimmed), "true") {
				return true
			}
		}
	}
	return false
}

// EnsureCodexSkillsFeature ensures [features] skills = true in Codex config.toml.
func EnsureCodexSkillsFeature(path string) (InstallAction, error) {
	existed := false
	existing := ""
	if data, err := os.ReadFile(path); err == nil {
		existed = true
		existing = string(data)
	} else if !os.IsNotExist(err) {
		return ActionError, err
	}
	if CodexSkillsFeatureEnabled(existing) {
		return ActionUnchanged, nil
	}

	var lines []string
	if existing != "" {
		lines = strings.Split(existing, "\n")
	}

	featuresIdx := -1
	skillsIdx := -1
	inFeatures := false
	for i, line := range lines {
		tableKey := strings.TrimSpace(strings.Split(line, "#")[0])
		if strings.HasPrefix(tableKey, "[") && strings.HasSuffix(tableKey, "]") {
			if tableKey == "[features]" {
				inFeatures = true
				featuresIdx = i
				continue
			}
			inFeatures = false
			continue
		}
		if inFeatures {
			parts := strings.Split(line, "#")
			if strings.Contains(strings.TrimSpace(parts[0]), "skills") {
				skillsIdx = i
			}
		}
	}

	writeLines := func() (InstallAction, error) {
		next := strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return ActionError, err
		}
		if err := os.WriteFile(path, []byte(next), 0o644); err != nil {
			return ActionError, err
		}
		if existed {
			return ActionUpdated, nil
		}
		return ActionCreated, nil
	}

	if featuresIdx >= 0 && skillsIdx >= 0 {
		lines[skillsIdx] = "skills = true"
		return writeLines()
	}
	if featuresIdx >= 0 {
		insert := append([]string{"skills = true"}, lines[featuresIdx+1:]...)
		lines = append(lines[:featuresIdx+1], insert...)
		return writeLines()
	}

	block := "[features]\nskills = true\n"
	base := strings.TrimRight(existing, "\n")
	var next string
	if base != "" {
		next = base + "\n\n" + block
	} else {
		next = block
	}
	if !strings.HasSuffix(next, "\n") {
		next += "\n"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return ActionError, err
	}
	if err := os.WriteFile(path, []byte(next), 0o644); err != nil {
		return ActionError, err
	}
	if existed {
		return ActionUpdated, nil
	}
	return ActionCreated, nil
}
