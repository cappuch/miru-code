package cliui

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/takara-ai/miru-code/internal/types"
	"github.com/takara-ai/miru-code/internal/utils"
)

// PrefersJSONOutput is true when --json was passed or stdout is not a TTY.
func PrefersJSONOutput(jsonFlag bool) bool {
	if jsonFlag {
		return true
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return true
	}
	return (fi.Mode() & os.ModeCharDevice) == 0
}

func colorEnabled(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func wrap(code, s string, w io.Writer) string {
	if !colorEnabled(w) {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

// Bold styles text.
func Bold(s string) string { return wrap("1", s, os.Stdout) }

// Cyan styles text.
func Cyan(s string) string { return wrap("36", s, os.Stdout) }

// Dim styles text.
func Dim(s string) string { return wrap("2", s, os.Stdout) }

// Green styles text.
func Green(s string) string { return wrap("32", s, os.Stderr) }

// Magenta styles text.
func Magenta(s string) string { return wrap("35", s, os.Stdout) }

// Red styles text.
func Red(s string) string { return wrap("31", s, os.Stderr) }

// Yellow styles text.
func Yellow(s string) string { return wrap("33", s, os.Stderr) }

// BrandTitle returns the MIRU brand string.
func BrandTitle() string {
	return wrap("1", "MIRU", os.Stderr) + wrap("2", " (見る)", os.Stderr)
}

// WriteStdout writes a line to stdout.
func WriteStdout(line string) {
	fmt.Fprintln(os.Stdout, line)
}

// WriteStderr writes a line to stderr.
func WriteStderr(line string) {
	fmt.Fprintln(os.Stderr, line)
}

// Divider prints a horizontal rule.
func Divider(char string, width int, w io.Writer) {
	if char == "" {
		char = "─"
	}
	if width <= 0 {
		width = 52
	}
	if w == nil {
		w = os.Stdout
	}
	fmt.Fprintln(w, Dim(strings.Repeat(char, width)))
}

// PrintBrandBanner prints a simple brand line (full ASCII art omitted in Go port).
func PrintBrandBanner(w io.Writer) {
	if w == nil {
		w = os.Stderr
	}
	fmt.Fprintln(w, BrandTitle())
}

// Header prints the help header.
func Header() {
	WriteStdout("")
	if colorEnabled(os.Stdout) {
		PrintBrandBanner(os.Stdout)
	} else {
		WriteStdout(BrandTitle())
	}
	Divider("", 0, os.Stdout)
}

// CommandHeader prints a command help header.
func CommandHeader(name, summary string) {
	WriteStdout("")
	if colorEnabled(os.Stdout) {
		PrintBrandBanner(os.Stdout)
		WriteStdout(summary)
	} else {
		WriteStdout(BrandTitle() + " " + name)
		WriteStdout(summary)
	}
	Divider("", 0, os.Stdout)
}

// Section prints a section title.
func Section(title string) {
	WriteStdout("")
	WriteStdout(Bold(title))
}

// CommandRow prints a command list row.
func CommandRow(name, description string) {
	pad := "  "
	nameCol := name + strings.Repeat(" ", max(0, 16-len(name)))
	WriteStdout(pad + Cyan(nameCol) + " " + Dim(description))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Success prints a success message.
func Success(message string) { fmt.Fprintln(os.Stderr, Green("✓ ")+message) }

// Info prints an info message.
func Info(message string) { fmt.Fprintln(os.Stderr, Dim("· ")+message) }

// Warn prints a warning.
func Warn(message string) { fmt.Fprintln(os.Stderr, Yellow("! ")+message) }

// Fail prints an error.
func Fail(message string) { fmt.Fprintln(os.Stderr, Red("✗ ")+message) }

// Hint prints a dim hint.
func Hint(message string) { fmt.Fprintln(os.Stderr, Dim("  "+message)) }

const previewLines = 12
const previewWidth = 72

func truncateLine(line string, width int) string {
	if len(line) <= width {
		return line
	}
	return line[:width-1] + "…"
}

func previewContent(content string) []string {
	lines := strings.Split(content, "\n")
	if len(lines) > previewLines {
		lines = lines[:previewLines]
	}
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = truncateLine(strings.ReplaceAll(line, "\t", "  "), previewWidth)
	}
	return out
}

// FormatSearchResultsPretty renders search hits for the terminal.
func FormatSearchResultsPretty(query string, results []types.SearchResult) string {
	var b strings.Builder
	label := fmt.Sprintf("%d results", len(results))
	if len(results) == 1 {
		label = "1 result"
	}
	b.WriteString("\n")
	b.WriteString(Bold(label) + Dim(" for ") + Cyan(`"`+query+`"`) + "\n\n")
	maxScore := 0.0
	for _, r := range results {
		if r.Score > maxScore {
			maxScore = r.Score
		}
	}
	for i, result := range results {
		chunk := result.Chunk
		location := fmt.Sprintf("%s:%d-%d", chunk.FilePath, chunk.StartLine, chunk.EndLine)
		lang := ""
		if chunk.Language != nil {
			lang = Dim("  " + *chunk.Language)
		}
		b.WriteString(fmt.Sprintf("%s %s  %s%s\n",
			Dim(fmt.Sprintf("[%d]", i+1)),
			Magenta(utils.FormatRelevanceScore(result.Score, maxScore)),
			Bold(location),
			lang,
		))
		b.WriteString(Dim(strings.Repeat("─", 52)) + "\n")
		for _, line := range previewContent(chunk.Content) {
			b.WriteString(Dim("  ") + line + "\n")
		}
		total := strings.Count(chunk.Content, "\n") + 1
		if total > previewLines {
			b.WriteString(Dim(fmt.Sprintf("  … %d more lines", total-previewLines)) + "\n")
		}
		if i < len(results)-1 {
			b.WriteString("\n")
		}
	}
	if colorEnabled(os.Stdout) {
		b.WriteString("\n" + Dim("Tip: add --json for machine-readable output") + "\n")
	}
	return b.String()
}

// FormatSearchErrorPretty formats an empty-result message.
func FormatSearchErrorPretty(message string) string {
	return "\n" + Yellow("No results") + " " + Dim("—") + " " + message + "\n"
}

// FormatRelatedHeader builds the find-related query label.
func FormatRelatedHeader(filePath string, line int) string {
	return fmt.Sprintf("related to %s:%d", filePath, line)
}
