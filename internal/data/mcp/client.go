// Package mcp adapts MCP servers to the Agent's LLM tool interface.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	protocol "github.com/mark3labs/mcp-go/mcp"

	"kratos-demo/internal/conf"

	lmm "kratos-demo/internal/biz/llm"

	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/responses"
	"github.com/openai/openai-go/shared/constant"
)

type Config struct {
	Name    string
	Command string
	Args    []string
	Env     []string
	Timeout time.Duration
}

type Client struct {
	config Config
	client *mcpclient.Client
	tools  []lmm.Tool
}

func NewClients(ctx context.Context, servers []*conf.Runtime_MCPServer) ([]*Client, []lmm.Tool, error) {
	clients := make([]*Client, 0, len(servers))
	tools := make([]lmm.Tool, 0)
	for _, server := range servers {
		if server == nil || strings.TrimSpace(server.GetCommand()) == "" {
			continue
		}
		env := make([]string, 0, len(server.GetEnv()))
		for _, item := range server.GetEnv() {
			if item != nil && strings.TrimSpace(item.GetKey()) != "" {
				env = append(env, item.GetKey()+"="+item.GetValue())
			}
		}
		client, discovered, err := NewClient(ctx, Config{Name: server.GetName(), Command: server.GetCommand(), Args: server.GetArgs(), Env: env, Timeout: time.Duration(server.GetTimeoutSeconds()) * time.Second})
		if err != nil {
			for _, opened := range clients {
				opened.Close()
			}
			return nil, nil, fmt.Errorf("connect MCP server %s: %w", server.GetName(), err)
		}
		clients = append(clients, client)
		tools = append(tools, discovered...)
	}
	return clients, tools, nil
}

func (c *Client) Tools() []lmm.Tool { return append([]lmm.Tool(nil), c.tools...) }

func NewClient(ctx context.Context, config Config) (*Client, []lmm.Tool, error) {
	if strings.TrimSpace(config.Command) == "" {
		return nil, nil, errors.New("mcp command is empty")
	}
	if config.Timeout <= 0 {
		config.Timeout = 30 * time.Second
	}

	// The library owns MCP framing while this factory preserves our workspace cwd.
	commandFunc := func(ctx context.Context, command string, env []string, args []string) (*exec.Cmd, error) {
		cmd := exec.CommandContext(ctx, command, args...)
		cmd.Dir = workspaceRoot()
		cmd.Env = append(os.Environ(), env...)
		return cmd, nil
	}
	client, err := mcpclient.NewStdioMCPClientWithOptions(
		config.Command,
		config.Env,
		config.Args,
		transport.WithCommandFunc(commandFunc),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("start MCP stdio client: %w", err)
	}
	c := &Client{config: config, client: client}
	if err := c.initialize(ctx); err != nil {
		c.Close()
		return nil, nil, err
	}
	tools, err := c.listTools(ctx)
	if err != nil {
		c.Close()
		return nil, nil, err
	}
	c.tools = tools
	return c, tools, nil
}

func workspaceRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func (c *Client) initialize(ctx context.Context) error {
	ctx, cancel := c.withTimeout(ctx)
	defer cancel()
	request := protocol.InitializeRequest{}
	request.Params.ProtocolVersion = protocol.LATEST_PROTOCOL_VERSION
	request.Params.ClientInfo = protocol.Implementation{Name: "kratos-demo", Version: "0.1.0"}
	if _, err := c.client.Initialize(ctx, request); err != nil {
		return fmt.Errorf("initialize MCP server: %w", err)
	}
	return nil
}

func (c *Client) listTools(ctx context.Context) ([]lmm.Tool, error) {
	ctx, cancel := c.withTimeout(ctx)
	defer cancel()
	result, err := c.client.ListTools(ctx, protocol.ListToolsRequest{})
	if err != nil {
		return nil, fmt.Errorf("list MCP tools: %w", err)
	}
	tools := make([]lmm.Tool, 0, len(result.Tools))
	for _, tool := range result.Tools {
		if strings.TrimSpace(tool.Name) == "" {
			return nil, errors.New("mcp tool missing name")
		}
		schema, err := json.Marshal(tool.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("encode schema for MCP tool %s: %w", tool.Name, err)
		}
		var parameters any
		if err := json.Unmarshal(schema, &parameters); err != nil {
			return nil, fmt.Errorf("decode schema for MCP tool %s: %w", tool.Name, err)
		}
		parameterSchema, ok := parameters.(map[string]any)
		if !ok {
			parameterSchema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		tools = append(tools, lmm.Tool{OfFunction: &responses.FunctionToolParam{
			Name:        tool.Name,
			Description: param.NewOpt(tool.Description),
			Parameters:  parameterSchema,
			Strict:      param.NewOpt(false),
			Type:        constant.Function("function"),
		}})
	}
	return tools, nil
}

func (c *Client) Call(ctx context.Context, name, input string) (string, error) {
	args := map[string]any{}
	input = strings.TrimSpace(input)
	if input != "" {
		if err := json.Unmarshal([]byte(input), &args); err != nil {
			args["query"] = input
		}
	}
	ctx, cancel := c.withTimeout(ctx)
	defer cancel()
	request := protocol.CallToolRequest{}
	request.Params.Name = name
	request.Params.Arguments = args
	result, err := c.client.CallTool(ctx, request)
	if err != nil {
		return "", fmt.Errorf("call MCP tool %s: %w", name, err)
	}
	return formatToolResult(result)
}

func (c *Client) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(ctx, c.config.Timeout)
}

func (c *Client) Close() {
	if c != nil && c.client != nil {
		_ = c.client.Close()
	}
}

func formatToolResult(result *protocol.CallToolResult) (string, error) {
	if result == nil {
		return "", errors.New("MCP tool returned no result")
	}
	parts := make([]string, 0, len(result.Content))
	for _, content := range result.Content {
		if text, ok := content.(protocol.TextContent); ok {
			parts = append(parts, text.Text)
		}
	}
	output := strings.Join(parts, "\n")
	if result.IsError {
		if output == "" {
			output = "MCP tool returned an error"
		}
		return output, errors.New(output)
	}
	return output, nil
}
