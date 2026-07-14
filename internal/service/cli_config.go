package service

import (
	"fmt"
	"strings"

	"kratos-demo/internal/conf"
	"kratos-demo/internal/consts/public"
)

func (c *CLIService) BindAIConfig(ai *conf.AI) {
	if c == nil {
		return
	}
	c.aiConfig = ai
}

func (c *CLIService) ensureAIConfig() error {
	if c == nil || c.aiConfig == nil {
		return fmt.Errorf("ai config is not available")
	}

	creds, err := LoadCLICredentials()
	if err != nil {
		return err
	}
	creds.syncToAI(c.aiConfig)
	if aiConfigReady(c.aiConfig) {
		return nil
	}
	return c.promptAIConfigSetup(creds)
}

func (c *CLIService) promptAIConfigSetup(creds *CLICredentials) error {
	if c.ui == nil {
		label := providerLabel(c.aiConfig)
		return fmt.Errorf("ai.%s.api_key / base_url 未配置，请使用 /config 设置",
			strings.ToLower(label))
	}

	label := providerLabel(c.aiConfig)

	c.ui.println("")
	c.ui.println(c.ui.yellow(fmt.Sprintf("  ⚙  首次使用需配置 %s API", label)))
	c.ui.println(c.ui.dim("  将保存到 .myagent/credentials.json（已在 .gitignore）"))
	c.ui.println("")

	if strings.TrimSpace(creds.APIKey) == "" {
		line, err := c.readConfigLine("  API Key › ")
		if err != nil {
			return err
		}
		creds.APIKey = strings.TrimSpace(line)
	}

	defaultURL := defaultBaseURLForProvider(effectiveProvider(c.aiConfig))
	if strings.TrimSpace(creds.BaseURL) == "" {
		c.ui.println(c.ui.dim("  默认 Base URL  ") + defaultURL)
		line, err := c.readConfigLine("  Base URL › ")
		if err != nil {
			return err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			line = defaultURL
		}
		creds.BaseURL = line
	}

	if strings.TrimSpace(creds.APIKey) == "" || strings.TrimSpace(creds.BaseURL) == "" {
		return fmt.Errorf("api_key 与 base_url 均为必填")
	}
	if err := SaveCLICredentials(creds); err != nil {
		return err
	}
	creds.syncToAI(c.aiConfig)
	c.ui.println(c.ui.green("  ✓  API 配置已保存"))
	c.ui.println("")
	return nil
}

func (c *CLIService) readConfigLine(prompt string) (string, error) {
	if c.lineReader != nil && c.lineReader.useRL {
		c.ui.println("")
		return c.lineReader.ReadLineWithPrompt(c.ui, prompt, c.permissionMode())
	}
	c.ui.printf("%s", prompt)
	var line string
	if _, err := fmt.Fscanln(c.in, &line); err != nil {
		return "", err
	}
	return line, nil
}

func (c *CLIService) handleConfigCommand(line string) bool {
	raw := strings.TrimSpace(line)
	lower := strings.ToLower(raw)
	if lower == "/config" {
		c.printAIConfigStatus()
		return true
	}
	if !strings.HasPrefix(lower, "/config ") {
		return false
	}

	args := strings.Fields(raw)
	if len(args) < 2 {
		c.printAIConfigUsage()
		return true
	}

	switch strings.ToLower(args[1]) {
	case "show", "status":
		c.printAIConfigStatus()
	case "api_key", "apikey", "key":
		if len(args) < 3 {
			c.ui.println(c.ui.dim("  用法: /config api_key <your-key>"))
			return true
		}
		c.updateAIConfig(strings.Join(args[2:], " "), "")
	case "base_url", "baseurl", "url":
		if len(args) < 3 {
			c.ui.println(c.ui.dim("  用法: /config base_url <https://.../v1>"))
			return true
		}
		c.updateAIConfig("", strings.Join(args[2:], " "))
	default:
		c.printAIConfigUsage()
	}
	return true
}

func (c *CLIService) printAIConfigUsage() {
	c.ui.println("")
	c.ui.println(c.ui.bold("  /config"))
	c.ui.println(c.ui.dim("  /config              ") + "查看当前 API 配置")
	c.ui.println(c.ui.dim("  /config api_key ...  ") + "设置 API Key 并持久化")
	c.ui.println(c.ui.dim("  /config base_url ... ") + "设置 Base URL 并持久化")
	c.ui.println("")
}

func (c *CLIService) printAIConfigStatus() {
	c.ui.println("")
	path, _ := cliCredentialsPath()
	c.ui.println(c.ui.bold("  API 配置"))
	c.ui.println(c.ui.dim("  provider  ") + effectiveProvider(c.aiConfig))
	maskedKey := "(未设置)"
	baseURL := ""
	switch effectiveProvider(c.aiConfig) {
	case public.ProviderDeepSeek:
		if c.aiConfig.Deepseek != nil {
			maskedKey = maskCLIAPIKey(c.aiConfig.Deepseek.GetApiKey())
			baseURL = strings.TrimSpace(c.aiConfig.Deepseek.GetBaseUrl())
		}
	default:
		if c.aiConfig.Openai != nil {
			maskedKey = maskCLIAPIKey(c.aiConfig.Openai.GetApiKey())
			baseURL = strings.TrimSpace(c.aiConfig.Openai.GetBaseUrl())
		}
	}
	c.ui.println(c.ui.dim("  api_key   ") + maskedKey)
	c.ui.println(c.ui.dim("  base_url  ") + baseURL)
	if path != "" {
		c.ui.println(c.ui.dim("  file      ") + path)
	}
	c.ui.println("")
}

func (c *CLIService) updateAIConfig(apiKey, baseURL string) {
	if c.aiConfig == nil {
		c.ui.printError(fmt.Errorf("ai config is not available"))
		return
	}

	creds, err := LoadCLICredentials()
	if err != nil {
		c.ui.printError(err)
		return
	}
	if v := strings.TrimSpace(apiKey); v != "" {
		creds.APIKey = v
	}
	if v := strings.TrimSpace(baseURL); v != "" {
		creds.BaseURL = v
	}
	if strings.TrimSpace(creds.APIKey) == "" || strings.TrimSpace(creds.BaseURL) == "" {
		c.ui.println(c.ui.red("  ✕  api_key 与 base_url 均需设置"))
		c.ui.println(c.ui.dim("  示例: /config base_url " + defaultBaseURLForProvider(effectiveProvider(c.aiConfig))))
		return
	}
	if err := SaveCLICredentials(creds); err != nil {
		c.ui.printError(err)
		return
	}
	creds.syncToAI(c.aiConfig)
	c.ui.println(c.ui.green("  ✓  已保存"))
	c.printAIConfigStatus()
}

// --- model command ---

type modelEntry struct {
	Name string
	Desc string
}

// availableModels 返回当前 provider 可选的模型列表。
func availableModels(ai *conf.AI) []modelEntry {
	switch effectiveProvider(ai) {
	case public.ProviderDeepSeek:
		return []modelEntry{
			{Name: "deepseek-v4-flash", Desc: "V4 Flash · 极速响应"},
			{Name: "deepseek-v4-pro", Desc: "V4 Pro · 最强推理"},
			{Name: "deepseek-chat", Desc: "V3 · 通用对话"},
			{Name: "deepseek-coder", Desc: "V3 · 代码生成"},
		}
	default:
		return []modelEntry{
			{Name: "gpt-4o-mini", Desc: "轻量快速 · 128K 上下文"},
			{Name: "gpt-4o", Desc: "标准模型 · 128K 上下文"},
			{Name: "gpt-4-turbo", Desc: "GPT-4 Turbo · 128K 上下文"},
		}
	}
}

// currentModel 返回当前 provider 正在使用的模型名。
func currentModel(ai *conf.AI) string {
	if ai == nil {
		return ""
	}
	switch effectiveProvider(ai) {
	case public.ProviderDeepSeek:
		if ai.Deepseek != nil {
			m := strings.TrimSpace(ai.Deepseek.GetModel())
			if m == "" {
				return "deepseek-v4-flash"
			}
			return m
		}
	default:
		if ai.Openai != nil {
			m := strings.TrimSpace(ai.Openai.GetModel())
			if m == "" {
				return "gpt-4o-mini"
			}
			return m
		}
	}
	return ""
}

// setModel 写入当前 provider 的模型名到 ai 配置中。
func setModel(ai *conf.AI, model string) {
	if ai == nil {
		return
	}
	switch effectiveProvider(ai) {
	case public.ProviderDeepSeek:
		if ai.Deepseek == nil {
			ai.Deepseek = &conf.AI_DeepSeek{}
		}
		ai.Deepseek.Model = model
	default:
		if ai.Openai == nil {
			ai.Openai = &conf.AI_OpenAI{}
		}
		ai.Openai.Model = model
	}
}

func (c *CLIService) showModelSelect() {
	models := availableModels(c.aiConfig)
	if len(models) == 0 {
		c.ui.println(c.ui.dim("  (无可用模型)"))
		return
	}
	curr := currentModel(c.aiConfig)

	c.ui.println("")
	c.ui.println(c.ui.bold("  model  ") + c.ui.dim("· "+effectiveProvider(c.aiConfig)))
	c.ui.println("")

	labels := make([]string, 0, len(models))
	defaultIdx := 0
	for i, entry := range models {
		label := entry.Name + "  " + c.ui.dim(entry.Desc)
		labels = append(labels, label)
		if strings.EqualFold(entry.Name, curr) {
			defaultIdx = i
		}
	}

	idx, err := promptSelect(c.out, c.in, c.ui.paint, labels, "  ↑↓ 选择 · Enter 确认 · Esc 取消", defaultIdx)
	if err != nil {
		c.ui.printError(err)
		return
	}
	if idx < 0 {
		c.ui.println(c.ui.dim("  已取消"))
		c.ui.println("")
		return
	}
	setModel(c.aiConfig, models[idx].Name)
	c.ui.println(c.ui.green("\n  ✓  已切换至 ") + c.ui.bold(models[idx].Name))
	c.ui.println(c.ui.dim("  请使用 /new 新建会话以应用新模型"))
	c.ui.println("")
}

func (c *CLIService) handleModelSwitch(modelName string) {
	modelName = strings.TrimSpace(modelName)
	for _, entry := range availableModels(c.aiConfig) {
		if strings.EqualFold(entry.Name, modelName) {
			setModel(c.aiConfig, entry.Name)
			c.ui.println(c.ui.green("  ✓  已切换至 ") + c.ui.bold(entry.Name))
			c.ui.println(c.ui.dim("  请使用 /new 新建会话以应用新模型"))
			c.ui.println("")
			return
		}
	}
	c.ui.println(c.ui.red("  ✕  未知模型: " + modelName))
	c.ui.println(c.ui.dim("  可用 /model 查看模型列表"))
	c.ui.println("")
}
