package file
import (
	"errors"
	"fmt"
	"strings"
)

// fileLineModel ???????????????????
type fileLineModel struct {
	lines           []string
	trailingNewline bool
}

func newFileLineModel(content string) fileLineModel {
	return fileLineModel{
		lines:           splitLinesPreserveTrailing(content),
		trailingNewline: strings.HasSuffix(content, "\n"),
	}
}

func (m fileLineModel) render() string {
	return joinLinesPreserveTrailing(m.lines, m.trailingNewline)
}

func (m fileLineModel) lineCount() int {
	return len(m.lines)
}

func normalizeLineSpan(start, end, total int) (int, int, error) {
	if start <= 0 {
		return 0, 0, errors.New("line must be >= 1")
	}
	if end <= 0 {
		end = start
	}
	if start > end {
		return 0, 0, fmt.Errorf("line %d cannot be greater than line_end %d", start, end)
	}
	if start > total {
		return 0, 0, fmt.Errorf("line %d exceeds file length (%d lines)", start, total)
	}
	if end > total {
		return 0, 0, fmt.Errorf("line_end %d exceeds file length (%d lines)", end, total)
	}
	return start, end, nil
}

func splitInsertLines(text string) []string {
	if text == "" {
		return []string{""}
	}
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	return strings.Split(normalized, "\n")
}

func ApplyPrepend(content, insertText string) (string, string, error) {
	updated, err := ApplyInsertLine(content, 1, insertText)
	if err != nil {
		return "", "", err
	}
	return updated, "prepended content at beginning of file", nil
}

func ApplyInsertLine(content string, line int, insertText string) (string, error) {
	model := newFileLineModel(content)
	if line > model.lineCount()+1 {
		return "", fmt.Errorf("line %d exceeds file length (%d lines); read_file to confirm line numbers", line, model.lineCount())
	}
	idx := line - 1
	chunk := splitInsertLines(insertText)
	model.lines = insertAt(model.lines, idx, chunk)
	return model.render(), nil
}

func ApplyInsertAfterLine(content string, line int, insertText string) (string, error) {
	model := newFileLineModel(content)
	if line > model.lineCount() {
		return "", fmt.Errorf("line %d exceeds file length (%d lines)", line, model.lineCount())
	}
	idx := line
	chunk := splitInsertLines(insertText)
	model.lines = insertAt(model.lines, idx, chunk)
	return model.render(), nil
}

func ApplyDeleteLine(content string, line int) (string, error) {
	return ApplyDeleteLines(content, line, line)
}

func ApplyDeleteLines(content string, start, end int) (string, error) {
	model := newFileLineModel(content)
	start, end, err := normalizeLineSpan(start, end, model.lineCount())
	if err != nil {
		return "", err
	}
	model.lines = deleteRange(model.lines, start-1, end-1)
	return model.render(), nil
}

func ApplyReplaceLine(content string, line int, newText string) (string, error) {
	return ApplyReplaceLines(content, line, line, newText)
}

func ApplyReplaceLines(content string, start, end int, newText string) (string, error) {
	model := newFileLineModel(content)
	start, end, err := normalizeLineSpan(start, end, model.lineCount())
	if err != nil {
		return "", err
	}
	replacement := splitInsertLines(newText)
	model.lines = replaceRange(model.lines, start-1, end-1, replacement)
	return model.render(), nil
}

func ApplyDeleteString(content, oldString string, replaceAll bool) (string, int, error) {
	return ApplySearchReplace(content, oldString, "", replaceAll)
}

func insertAt(lines []string, idx int, chunk []string) []string {
	if idx < 0 {
		idx = 0
	}
	if idx > len(lines) {
		idx = len(lines)
	}
	updated := make([]string, 0, len(lines)+len(chunk))
	updated = append(updated, lines[:idx]...)
	updated = append(updated, chunk...)
	updated = append(updated, lines[idx:]...)
	return updated
}

func deleteRange(lines []string, startIdx, endIdx int) []string {
	if len(lines) == 0 || startIdx >= len(lines) {
		return lines
	}
	if startIdx < 0 {
		startIdx = 0
	}
	if endIdx >= len(lines) {
		endIdx = len(lines) - 1
	}
	if startIdx > endIdx {
		return lines
	}
	updated := make([]string, 0, len(lines)-(endIdx-startIdx+1))
	updated = append(updated, lines[:startIdx]...)
	updated = append(updated, lines[endIdx+1:]...)
	return updated
}

func replaceRange(lines []string, startIdx, endIdx int, replacement []string) []string {
	if startIdx < 0 {
		startIdx = 0
	}
	if endIdx >= len(lines) {
		endIdx = len(lines) - 1
	}
	if startIdx > endIdx || len(lines) == 0 {
		return lines
	}
	updated := make([]string, 0, len(lines)-((endIdx-startIdx)+1)+len(replacement))
	updated = append(updated, lines[:startIdx]...)
	updated = append(updated, replacement...)
	updated = append(updated, lines[endIdx+1:]...)
	return updated
}

func splitLinesPreserveTrailing(content string) []string {
	if content == "" {
		return []string{}
	}
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	return strings.Split(normalized, "\n")
}

func joinLinesPreserveTrailing(lines []string, trailingNewline bool) string {
	joined := strings.Join(lines, "\n")
	if trailingNewline && !strings.HasSuffix(joined, "\n") {
		return joined + "\n"
	}
	return joined
}

func ApplySearchReplace(content, oldString, newString string, replaceAll bool) (string, int, error) {
	if oldString == "" {
		return "", 0, errors.New("old_string is required")
	}
	count := strings.Count(content, oldString)
	if count == 0 {
		return "", 0, errors.New("old_string not found in file; read_file first and copy an exact unique snippet")
	}
	if !replaceAll && count > 1 {
		return "", 0, fmt.Errorf("old_string matches %d times; make it unique or set replace_all=true", count)
	}
	if replaceAll {
		return strings.ReplaceAll(content, oldString, newString), count, nil
	}
	return strings.Replace(content, oldString, newString, 1), 1, nil
}

func ApplyAppendContent(content, appendText string) string {
	if content == "" {
		return appendText
	}
	if strings.HasSuffix(content, "\n") || strings.HasPrefix(appendText, "\n") {
		return content + appendText
	}
	return content + "\n" + appendText
}
