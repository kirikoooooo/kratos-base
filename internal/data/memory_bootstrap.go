package data

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kratos-demo/internal/biz"
	toolcatalog "kratos-demo/third_party/tools"
)

// BootstrapUserMemoryIfEmpty 从工作区发现工具与 skills，初始化用户级提示词记忆。
func BootstrapUserMemoryIfEmpty(ctx context.Context, store biz.AgentMemoryStore, userID, workspace string) error {
	if store == nil {
		return nil
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		userID = defaultMemoryUserID
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
		user = &biz.UserAgentMemory{UserID: userID}
	}
	if !needsUserBootstrap(user) {
		return nil
	}

	user.WorkspaceRoot = workspace
	user.ToolHints = discoverToolHints()
	user.SkillHints = discoverSkillHints(workspace)
	user.CommandPolicies = defaultCommandPolicies()
	user.PromptNotes = []biz.PromptAdjustment{{
		Title:   "记忆用途",
		Content: "以下内容用于提示词智能化调整，不包含对话历史；请结合当前任务选择性采纳。",
	}, {
		Title:   "项目类型",
		Content: "Kratos + Go 通用 agent 底座；优先通过工具读取/修改仓库并给出中文总结。",
	}}
	user.UpdatedAt = time.Now()
	return store.SaveUser(ctx, user)
}

func needsUserBootstrap(user *biz.UserAgentMemory) bool {
	if user == nil {
		return true
	}
	return len(user.ToolHints) == 0 && len(user.SkillHints) == 0 && len(user.CommandPolicies) == 0
}

func discoverToolHints() []biz.ToolHint {
	catalog, err := toolcatalog.DefaultCatalog()
	if err != nil || catalog == nil {
		return defaultToolHints()
	}
	names := []string{"read_file", "write_file", "exec_command"}
	hints := make([]biz.ToolHint, 0, len(names))
	for _, name := range names {
		def, ok := catalog.Lookup(name)
		if !ok {
			continue
		}
		hints = append(hints, biz.ToolHint{
			Name:      name,
			WhenToUse: strings.TrimSpace(def.Function.Description),
		})
	}
	if len(hints) == 0 {
		return defaultToolHints()
	}
	return hints
}

func defaultToolHints() []biz.ToolHint {
	return []biz.ToolHint{
		{Name: "read_file", WhenToUse: "查看 README、配置、源码与目录结构", Constraints: "路径相对于工作区根目录"},
		{Name: "write_file", WhenToUse: "在确认方案后修改或新增文件", Constraints: "不要编造未写入的内容"},
		{Name: "exec_command", WhenToUse: "编译、测试或定位文件", Constraints: "全任务最多 3 次；优先 read_file"},
	}
}

func defaultCommandPolicies() []biz.CommandPolicy {
	return []biz.CommandPolicy{
		{
			Situation: "理解项目与启动方式",
			Commands:  []string{"read_file README.md", "read_file configs/config.yaml"},
			Notes:     "先理解仓库再改代码",
		},
		{
			Situation: "验证 Go 工程可编译",
			Commands:  []string{"exec_command go build ./..."},
		},
		{
			Situation: "read_file 因路径不存在失败",
			Commands:  []string{"exec_command dir /s /b <pattern>"},
			Notes:     "修正路径后再 read_file；exec_command 全任务最多 3 次",
		},
		{
			Situation: "需要系统化排查工具/任务失败",
			Commands:  []string{"参考 skill: debugging/systematic-debugging"},
			Notes:     "见 .agents/skills 下对应 SKILL.md",
		},
	}
}

func discoverSkillHints(workspace string) []biz.SkillHint {
	root := filepath.Join(workspace, ".agents", "skills")
	var skillHints []biz.SkillHint
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
		skillHints = append(skillHints, biz.SkillHint{
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
		return "任务失败、工具报错或结果不符合预期时"
	case "test-driven-development":
		return "需要补充或调整测试时"
	case "verification-before-completion":
		return "声称完成前需要运行验证命令时"
	case "executing-plans":
		return "按实施计划批量执行任务时"
	default:
		return "与当前任务场景匹配时可参考对应 SKILL.md"
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
