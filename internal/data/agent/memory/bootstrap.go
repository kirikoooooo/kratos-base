package memory
import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	toolcatalog "kratos-demo/third_party/tools"
	"kratos-demo/internal/data/common"
)

// BootstrapUserMemoryIfEmpty ?????????? skills/?????
func BootstrapUserMemoryIfEmpty(ctx context.Context, store AgentMemoryStore, userID, workspace string) error {
	if store == nil {
		return nil
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		userID = common.DefaultMemoryUserID
	}
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		var err error
		workspace, err = os.Getwd()
		if err != nil {
			return err
		}
	}

	user, err := store.LoadUser(ctx, userID)
	if err != nil {
		return err
	}
	if user == nil {
		user = &UserAgentMemory{UserID: userID}
	}
	if !needsUserBootstrap(user) {
		return nil
	}

	user.WorkspaceRoot = workspace
	user.ToolHints = discoverToolHints()
	user.SkillHints = discoverSkillHints(workspace)
	user.CommandPolicies = defaultCommandPolicies()
	user.PromptNotes = []PromptAdjustment{{
		Title:   "????",
		Content: "????????????????????????????????????",
	}, {
		Title:   "????",
		Content: "Kratos + Go ?? agent ???????????/????????????",
	}}
	user.UpdatedAt = time.Now()
	return store.SaveUser(ctx, user)
}

func needsUserBootstrap(user *UserAgentMemory) bool {
	if user == nil {
		return true
	}
	return len(user.ToolHints) == 0 && len(user.SkillHints) == 0 && len(user.CommandPolicies) == 0
}

func discoverToolHints() []ToolHint {
	catalog, err := toolcatalog.DefaultCatalog()
	if err != nil || catalog == nil {
		return defaultToolHints()
	}
	names := []string{"read_file", "edit_file", "write_file", "delete_file", "exec_command"}
	hints := make([]ToolHint, 0, len(names))
	for _, name := range names {
		def, ok := catalog.Lookup(name)
		if !ok {
			continue
		}
		hints = append(hints, ToolHint{
			Name:      name,
			WhenToUse: strings.TrimSpace(def.Function.Description),
		})
	}
	if len(hints) == 0 {
		return defaultToolHints()
	}
	return hints
}

func defaultToolHints() []ToolHint {
	return []ToolHint{
		{Name: "read_file", WhenToUse: "?? README????????????? internal/biz?? read_file ?????", Constraints: "????????????????? entry ??"},
		{Name: "edit_file", WhenToUse: "?????????(insert_line/insert_after_line/prepend/append)???(delete_line/delete_lines/delete_string)???(replace_line/replace_lines/search_replace)", Constraints: "?????? read_file????????? append"},
		{Name: "write_file", WhenToUse: "??????????????", Constraints: "??????????"},
		{Name: "delete_file", WhenToUse: "???????????????????", Constraints: "????????????? edit_file ?? exec rm"},
		{Name: "exec_command", WhenToUse: "??????????", Constraints: "????? 3 ???? read_file?rm/del ?? CLI ??"},
	}
}

func defaultCommandPolicies() []CommandPolicy {
	return []CommandPolicy{
		{
			Situation: "?????????",
			Commands:  []string{"read_file README.md", "read_file configs/config.yaml"},
			Notes:     "?????????",
		},
		{
			Situation: "?? Go ?????",
			Commands:  []string{"exec_command go build ./..."},
		},
		{
			Situation: "read_file ????????",
			Commands:  []string{"read_file internal/biz", "read_file internal"},
			Notes:     "? read_file ??????????? exec_command?exec ????? 3 ?",
		},
		{
			Situation: "?????????/????",
			Commands:  []string{"?? skill: debugging/systematic-debugging"},
			Notes:     "? .agents/skills ??? SKILL.md",
		},
	}
}

func discoverSkillHints(workspace string) []SkillHint {
	root := filepath.Join(workspace, ".agents", "skills")
	var skillHints []SkillHint
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() || d.Name() != "SKILL.md" {
			return nil
		}
		rel, relErr := filepath.Rel(workspace, path)
		if relErr != nil {
			rel = path
		}
		name := skillNameFromPath(rel)
		desc := readSkillSummary(path)
		skillHints = append(skillHints, SkillHint{
			Name:        name,
			Path:        filepath.ToSlash(rel),
			WhenToUse:   skillWhenToUse(name),
			Description: desc,
		})
		return nil
	})
	if len(skillHints) > 24 {
		skillHints = skillHints[:24]
	}
	return skillHints
}

func skillNameFromPath(rel string) string {
	rel = filepath.ToSlash(rel)
	parts := strings.Split(rel, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-2]
	}
	return strings.TrimSuffix(filepath.Base(rel), ".md")
}

func skillWhenToUse(name string) string {
	switch strings.ToLower(name) {
	case "systematic-debugging":
		return "??????????????????"
	case "test-driven-development":
		return "??????????"
	case "verification-before-completion":
		return "??????????????"
	case "executing-plans":
		return "????????????"
	default:
		return "??????????????? SKILL.md"
	}
}

func readSkillSummary(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(raw), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "---") {
			continue
		}
		if len(line) > 160 {
			return line[:160] + "..."
		}
		return line
	}
	return ""
}
