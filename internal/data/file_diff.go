package data

import (
	"fmt"
	"strings"
)

func formatUnifiedDiff(path, before, after string) string {
	if before == after {
		return ""
	}
	beforeLines := splitDiffLines(before)
	afterLines := splitDiffLines(after)

	prefix, beforeMid, afterMid, suffix := splitCommonAffix(beforeLines, afterLines)
	if len(beforeMid) == 0 && len(afterMid) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("--- ")
	b.WriteString(path)
	b.WriteString(" (session start)\n")
	b.WriteString("+++ ")
	b.WriteString(path)
	b.WriteString(" (current)\n")
	if len(prefix) > 0 {
		fmt.Fprintf(&b, "@@ context prefix %d lines @@\n", len(prefix))
		for _, line := range prefix {
			b.WriteString(" ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	fmt.Fprintf(&b, "@@ change @@\n")
	for _, line := range beforeMid {
		b.WriteString("-")
		b.WriteString(line)
		b.WriteString("\n")
	}
	for _, line := range afterMid {
		b.WriteString("+")
		b.WriteString(line)
		b.WriteString("\n")
	}
	if len(suffix) > 0 {
		fmt.Fprintf(&b, "@@ context suffix %d lines @@\n", len(suffix))
		for _, line := range suffix {
			b.WriteString(" ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func splitDiffLines(content string) []string {
	if content == "" {
		return nil
	}
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func splitCommonAffix(before, after []string) (prefix, beforeMid, afterMid, suffix []string) {
	i := 0
	for i < len(before) && i < len(after) && before[i] == after[i] {
		i++
	}
	prefix = append([]string(nil), before[:i]...)
	before = before[i:]
	after = after[i:]

	j := 0
	for j < len(before) && j < len(after) && before[len(before)-1-j] == after[len(after)-1-j] {
		j++
	}
	if j > 0 {
		suffix = append([]string(nil), before[len(before)-j:]...)
		before = before[:len(before)-j]
		after = after[:len(after)-j]
	}
	return prefix, before, after, suffix
}
