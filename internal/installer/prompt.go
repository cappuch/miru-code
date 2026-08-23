package installer

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/takara-ai/miru-code/internal/cliui"
	"golang.org/x/term"
)

const (
	esc         = "\x1b"
	hideCursor  = esc + "[?25l"
	showCursor  = esc + "[?25h"
	clearLine   = esc + "[2K"
	cursorStart = esc + "[G"
)

// MultiSelectItem is one row in an interactive multi-select.
type MultiSelectItem[T any] struct {
	Label   string
	Value   T
	Checked bool
}

func isTTY() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// PromptConfirm asks a yes/no question with arrow-key TUI when interactive.
func PromptConfirm(question string, defaultYes bool) bool {
	if !isTTY() {
		suffix := " [Y/n] "
		if !defaultYes {
			suffix = " [y/N] "
		}
		fmt.Fprint(os.Stderr, question+suffix)
		scanner := bufio.NewScanner(os.Stdin)
		if !scanner.Scan() {
			return defaultYes
		}
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		if answer == "" {
			return defaultYes
		}
		return answer == "y" || answer == "yes"
	}

	yesSelected := defaultYes
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return defaultYes
	}
	defer func() {
		_ = term.Restore(int(os.Stdin.Fd()), oldState)
		fmt.Fprint(os.Stderr, showCursor)
	}()
	fmt.Fprint(os.Stderr, hideCursor)
	writeConfirm(question, yesSelected)

	buf := make([]byte, 8)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil || n == 0 {
			return false
		}
		key := parseInstallerKey(buf[:n])
		switch key {
		case "ctrl-c":
			fmt.Fprint(os.Stderr, "\r\n")
			return false
		case "enter":
			fmt.Fprint(os.Stderr, "\r\n")
			return yesSelected
		case "left", "right", "yes", "no":
			next := nextConfirmYesSelected(yesSelected, key)
			if next != yesSelected {
				yesSelected = next
				rewriteConfirm(question, yesSelected)
			}
		}
	}
}

// nextConfirmYesSelected maps confirm keys to the next Yes selection.
// Left/← and y → Yes; right/→ and n → No (matches on-screen Yes | No order).
func nextConfirmYesSelected(yesSelected bool, key string) bool {
	if key == "left" || key == "yes" {
		return true
	}
	if key == "right" || key == "no" {
		return false
	}
	return yesSelected
}

func writeConfirm(question string, yesSelected bool) {
	fmt.Fprint(os.Stderr, strings.Join(buildConfirmLines(question, yesSelected), "\r\n")+"\r\n")
}

func rewriteConfirm(question string, yesSelected bool) {
	// Move up 2 lines and rewrite prompt line.
	fmt.Fprint(os.Stderr, esc+"[2A"+cursorStart+clearLine+buildConfirmPromptLine(question, yesSelected)+"\r\n")
	fmt.Fprint(os.Stderr, cursorStart+clearLine+cliui.Dim("←→ or y/n to choose  enter to confirm  ctrl-c to cancel")+"\r\n")
}

func buildConfirmPromptLine(question string, yesSelected bool) string {
	yes := cliui.Dim(" Yes ")
	no := cliui.Dim(" No ")
	if yesSelected {
		yes = cliui.Green(cliui.Bold(" Yes "))
	} else {
		no = cliui.Green(cliui.Bold(" No "))
	}
	return question + "  " + yes + " / " + no
}

func buildConfirmLines(question string, yesSelected bool) []string {
	return []string{
		buildConfirmPromptLine(question, yesSelected),
		cliui.Dim("←→ or y/n to choose  enter to confirm  ctrl-c to cancel"),
	}
}

// RequireInteractiveTerminal errors when stdin is not a TTY unless non-interactive flags/env set.
func RequireInteractiveTerminal(command string) error {
	if NonInteractiveInstall() {
		return nil
	}
	if isTTY() {
		return nil
	}
	return fmt.Errorf("%s requires an interactive terminal (or set MIRU_INSTALL_AGENTS / --yes --all)", command)
}

// NonInteractiveInstall reports env/flag-driven non-interactive mode.
func NonInteractiveInstall() bool {
	v := strings.TrimSpace(os.Getenv("MIRU_INSTALL_NONINTERACTIVE"))
	if v == "1" || strings.EqualFold(v, "true") {
		return true
	}
	if strings.TrimSpace(os.Getenv("MIRU_INSTALL_AGENTS")) != "" {
		return true
	}
	return false
}

// ParseInstallAgentsEnv parses MIRU_INSTALL_AGENTS (comma-separated ids or "all").
func ParseInstallAgentsEnv() []string {
	raw := strings.TrimSpace(os.Getenv("MIRU_INSTALL_AGENTS"))
	if raw == "" {
		return nil
	}
	if strings.EqualFold(raw, "all") {
		ids := make([]string, 0)
		for _, a := range AgentTargets() {
			ids = append(ids, a.ID)
		}
		return ids
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// PromptMultiSelect runs an interactive multi-select (↑↓ space a enter).
// Returns nil on cancel (ctrl-c).
func PromptMultiSelect[T any](title string, items []MultiSelectItem[T]) []T {
	if len(items) == 0 {
		return []T{}
	}
	if NonInteractiveInstall() || !isTTY() {
		out := make([]T, 0)
		for _, item := range items {
			if item.Checked {
				out = append(out, item.Value)
			}
		}
		return out
	}

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return PromptMultiSelectSimplified(title, toSimplified(items))
	}
	defer func() {
		_ = term.Restore(int(os.Stdin.Fd()), oldState)
		fmt.Fprint(os.Stderr, showCursor)
		_ = syscall.SetNonblock(int(os.Stdin.Fd()), false)
	}()

	fmt.Fprint(os.Stderr, hideCursor)
	cursor := 0
	footer := "↑↓ move  space toggle  a all  enter confirm  ctrl-c cancel"
	fmt.Fprint(os.Stderr, strings.Join(buildMultiSelectLines(title, items, cursor, footer), "\r\n")+"\r\n")

	buf := make([]byte, 8)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil || n == 0 {
			fmt.Fprint(os.Stderr, "\r\n")
			return nil
		}
		key := parseInstallerKey(buf[:n])
		switch key {
		case "ctrl-c":
			fmt.Fprint(os.Stderr, "\r\n")
			return nil
		case "enter":
			fmt.Fprint(os.Stderr, "\r\n")
			out := make([]T, 0)
			for _, item := range items {
				if item.Checked {
					out = append(out, item.Value)
				}
			}
			return out
		case "up":
			if cursor > 0 {
				prev := cursor
				cursor--
				updateMultiSelectRow(items, prev, cursor, len(items))
			}
		case "down":
			if cursor < len(items)-1 {
				prev := cursor
				cursor++
				updateMultiSelectRow(items, prev, cursor, len(items))
			}
		case "space":
			items[cursor].Checked = !items[cursor].Checked
			rewriteItemRow(items[cursor], cursor, len(items), true)
		case "all":
			allChecked := true
			for _, item := range items {
				if !item.Checked {
					allChecked = false
					break
				}
			}
			for i := range items {
				items[i].Checked = !allChecked
				rewriteItemRow(items[i], i, len(items), i == cursor)
			}
		}
	}
}

// PromptMultiSelectSimplified picks values via per-item confirm (legacy fallback).
func PromptMultiSelectSimplified[T any](label string, items []struct {
	Label   string
	Value   T
	Checked bool
}) []T {
	cliui.Info(label)
	if NonInteractiveInstall() || !isTTY() {
		out := make([]T, 0)
		for _, item := range items {
			if item.Checked {
				out = append(out, item.Value)
			}
		}
		return out
	}
	out := make([]T, 0)
	for _, item := range items {
		if PromptConfirm(item.Label, item.Checked) {
			out = append(out, item.Value)
		}
	}
	return out
}

func toSimplified[T any](items []MultiSelectItem[T]) []struct {
	Label   string
	Value   T
	Checked bool
} {
	out := make([]struct {
		Label   string
		Value   T
		Checked bool
	}, len(items))
	for i, item := range items {
		out[i].Label = item.Label
		out[i].Value = item.Value
		out[i].Checked = item.Checked
	}
	return out
}

func buildMultiSelectLines[T any](title string, items []MultiSelectItem[T], cursor int, footer string) []string {
	lines := []string{"", cliui.Bold(title), cliui.Dim(strings.Repeat("─", 48))}
	for i, item := range items {
		lines = append(lines, formatMultiSelectItemLine(item.Label, item.Checked, i == cursor))
	}
	lines = append(lines, "", cliui.Dim(footer))
	return lines
}

func formatMultiSelectItemLine(label string, checked, selected bool) string {
	pointer := " "
	if selected {
		pointer = cliui.Cyan("›")
	}
	mark := cliui.Dim("[ ]")
	if checked {
		mark = cliui.Cyan("[x]")
	}
	lab := label
	if selected {
		lab = cliui.Bold(label)
	}
	return "  " + pointer + " " + mark + " " + lab
}

const (
	multiSelectHeaderLines  = 3
	multiSelectTrailerLines = 2
)

func multiSelectTotalLines(itemCount int) int {
	return multiSelectHeaderLines + itemCount + multiSelectTrailerLines
}

func multiSelectItemOffsetFromBottom(itemIndex, itemCount int) int {
	return multiSelectTotalLines(itemCount) - (multiSelectHeaderLines + itemIndex)
}

func rewriteItemRow[T any](item MultiSelectItem[T], itemIndex, itemCount int, selected bool) {
	offset := multiSelectItemOffsetFromBottom(itemIndex, itemCount)
	fmt.Fprintf(os.Stderr, "%s[%dA%s%s%s", esc, offset, cursorStart, clearLine, formatMultiSelectItemLine(item.Label, item.Checked, selected))
	fmt.Fprintf(os.Stderr, "%s[%dB", esc, offset)
}

func updateMultiSelectRow[T any](items []MultiSelectItem[T], prev, curr, itemCount int) {
	rewriteItemRow(items[prev], prev, itemCount, false)
	rewriteItemRow(items[curr], curr, itemCount, true)
}

// ParseInstallerKeyForTest exposes key parsing for tests (matches parseInstallerKeyForTest).
func ParseInstallerKeyForTest(chunk []byte) string {
	return parseInstallerKey(chunk)
}

func parseInstallerKey(chunk []byte) string {
	text := string(chunk)
	if arrow := parseArrowKey(chunk, text); arrow != "" {
		return arrow
	}
	if text == "\r" || text == "\n" {
		return "enter"
	}
	if text == " " {
		return "space"
	}
	if text == "\x03" {
		return "ctrl-c"
	}
	if text == "a" || text == "A" {
		return "all"
	}
	if text == "y" || text == "Y" {
		return "yes"
	}
	if text == "n" || text == "N" {
		return "no"
	}
	return text
}

func parseArrowKey(chunk []byte, text string) string {
	if len(chunk) >= 2 && (chunk[0] == 0x00 || chunk[0] == 0xe0) {
		switch chunk[1] {
		case 0x48:
			return "up"
		case 0x50:
			return "down"
		case 0x4d:
			return "right"
		case 0x4b:
			return "left"
		}
	}
	if strings.HasPrefix(text, esc) && len(text) >= 3 {
		intro := text[1]
		if intro == '[' || intro == 'O' {
			arrow := text[len(text)-1]
			switch arrow {
			case 'A':
				return "up"
			case 'B':
				return "down"
			case 'C':
				return "right"
			case 'D':
				return "left"
			}
		}
	}
	return ""
}
