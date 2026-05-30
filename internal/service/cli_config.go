package service

import (
	"fmt"
	"strings"

	"kratos-demo/internal/conf"
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
		return fmt.Errorf("ai.openai.api_key / base_url 未配置，请使用 /config 设置")
	}

	c.ui.println("")
	c.ui.println(c.ui.yellow("  ⚙  首次使用需配置 OpenAI 兼容 API"))
	c.ui.println(c.ui.dim("  将保存到 .myagent/credentials.json（已在 .gitignore）"))
	c.ui.println("")

	if strings.TrimSpace(creds.APIKey) == "" {
		line, err := c.readConfigLine("  API Key › ")
		if err != nil {
			return err
		}
		creds.APIKey = strings.TrimSpace(line)
	}
	if strings.TrimSpace(creds.BaseURL) == "" {
		c.ui.println(c.ui.dim("  默认 Base URL  ") + defaultCLIAPIBaseURL)
		line, err := c.readConfigLine("  Base URL › ")
		if err != nil {
			return err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			line = defaultCLIAPIBaseURL
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
	if c.aiConfig != nil && c.aiConfig.Openai != nil {
		c.ui.println(c.ui.dim("  api_key   ") + maskCLIAPIKey(c.aiConfig.Openai.GetApiKey()))
		c.ui.println(c.ui.dim("  base_url  ") + strings.TrimSpace(c.aiConfig.Openai.GetBaseUrl()))
	}
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
		c.ui.println(c.ui.dim("  示例: /config base_url " + defaultCLIAPIBaseURL))
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
