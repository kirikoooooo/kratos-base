from pathlib import Path

p = Path(__file__).resolve().parents[1] / "internal/data/agent/session_error_helpers_test.go"
want = "".join(chr(c) for c in [0x672C, 0x4F1A, 0x8BDD, 0x8FD1, 0x671F, 0x9519, 0x8BEF])
lines = [
    "package agent",
    "",
    "import (",
    '\t"strings"',
    '\t"testing"',
    '\t"time"',
    "",
    '\t"kratos-demo/internal/biz"',
    ")",
    "",
    "func TestFormatSessionErrorsForPrompt(t *testing.T) {",
    "\trecords := []biz.SessionErrorRecord{{",
    "\t\tTime:      time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC),",
    '\t\tSessionID: "dashboard-router",',
    '\t\tAgent:     "router",',
    '\t\tStage:     "tool_error",',
    '\t\tTool:      "read_file",',
    '\t\tMessage:   "file not found",',
    '\t\tDetail:    "path=internal/missing.go",',
    "\t}}",
    '\tgot := formatSessionErrorsForPrompt(records, ".myagent", "dashboard-router")',
    "\tfor _, want := range []string{",
    f'\t\t"{want}",',
    '\t\t"dashboard-router.jsonl",',
    '\t\t"tool_error",',
    '\t\t"read_file",',
    '\t\t"file not found",',
    '\t\t"path=internal/missing.go",',
    "\t} {",
    "\t\tif !strings.Contains(got, want) {",
    '\t\t\tt.Fatalf("missing %q in:\\n%s", want, got)',
    "\t\t}",
    "\t}",
    "}",
    "",
]
p.write_text("\n".join(lines), encoding="utf-8", newline="\n")
assert want == "\u672c\u4f1a\u8bdd\u8fd1\u671f\u9519\u8bef"
print("written", p)
