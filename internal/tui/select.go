// Package tui implements an arrow-key selector rendered on stderr,
// keeping stdout clean for machine-readable output (wt _path).
package tui

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"golang.org/x/term"
)

// Item pairs a display label with the value returned on selection.
type Item struct {
	Label string
	Value string
}

var ansiRE = regexp.MustCompile("\x1b\\[[^m]*m")

func visibleLen(s string) int {
	return len(ansiRE.ReplaceAllString(s, ""))
}

func truncate(s string, width int) string {
	if visibleLen(s) <= width {
		return s
	}
	vis, cut := 0, 0
	i := 0
	for i < len(s) && vis < width-1 {
		if s[i] == 0x1b {
			j := i + 1
			for j < len(s) && s[j] != 'm' {
				j++
			}
			i = j + 1
		} else {
			vis++
			i++
		}
		cut = i
	}
	return s[:cut] + "\x1b[0m"
}

func writeStderr(s string) {
	os.Stderr.WriteString(s)
}

func termWidth() int {
	w, _, err := term.GetSize(int(os.Stderr.Fd()))
	if err != nil || w <= 0 {
		return 80
	}
	return w
}

// ArrowSelect renders an interactive selector on stderr and returns the
// selected value, or "" when cancelled. Falls back to a numbered list
// when stderr is not a TTY.
func ArrowSelect(items []Item, title string, defaultIndex int) (string, bool) {
	if len(items) == 0 {
		return "", false
	}
	if defaultIndex < 0 {
		defaultIndex = 0
	}
	if defaultIndex >= len(items) {
		defaultIndex = len(items) - 1
	}
	if !term.IsTerminal(int(os.Stderr.Fd())) || !term.IsTerminal(int(os.Stdin.Fd())) {
		return fallbackSelect(items, title, defaultIndex)
	}
	return rawSelect(items, title, defaultIndex)
}

func render(items []Item, title string, selected int, firstRender bool) {
	width := termWidth()
	if !firstRender {
		writeStderr("\x1b[u")
	}
	writeStderr("\x1b[s")
	line := fmt.Sprintf("  \x1b[1m%s\x1b[0m", title)
	writeStderr("\x1b[2K" + truncate(line, width) + "\r\n")
	writeStderr("\x1b[2K\r\n")
	for i, it := range items {
		writeStderr("\x1b[2K")
		var line string
		if i == selected {
			line = fmt.Sprintf("  \x1b[1;7m > %s \x1b[0m  \x1b[2m%s\x1b[0m", it.Label, it.Value)
		} else {
			line = fmt.Sprintf("    %s  \x1b[2m%s\x1b[0m", it.Label, it.Value)
		}
		writeStderr(truncate(line, width) + "\r\n")
	}
	// Clear leftover lines below, then move back up.
	for i := 0; i < 2; i++ {
		writeStderr("\x1b[2K\r\n")
	}
	writeStderr("\x1b[2A")
}

func cleanup(totalLines int) {
	writeStderr("\x1b[u")
	for i := 0; i < totalLines+2; i++ {
		writeStderr("\x1b[2K\r\n")
	}
	writeStderr("\x1b[u")
}

// fallbackSelect prints a numbered list and reads a choice from stdin.
func fallbackSelect(items []Item, title string, defaultIndex int) (string, bool) {
	out := os.Stderr
	fmt.Fprintf(out, "\n  %s\n\n", title)
	for i, it := range items {
		marker := " "
		if i == defaultIndex {
			marker = ">"
		}
		fmt.Fprintf(out, "  %s [%d] %s  %s\n", marker, i+1, it.Label, it.Value)
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Select [1-%d]: ", len(items))
	var input string
	if _, err := fmt.Fscanln(os.Stdin, &input); err != nil {
		return items[defaultIndex].Value, true
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return items[defaultIndex].Value, true
	}
	var idx int
	if _, err := fmt.Sscanf(input, "%d", &idx); err == nil && idx >= 1 && idx <= len(items) {
		return items[idx-1].Value, true
	}
	return "", false
}
