// Package mcp provides a small stdio JSON-RPC client for MCP tool servers.
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"kratos-demo/internal/conf"

	lmm "kratos-demo/internal/biz/llm"
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
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	output *bufio.Reader
	stderr *bytes.Buffer
	mu     sync.Mutex
	nextID int64
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
		client.tools = discovered
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
	cmd := exec.Command(config.Command, config.Args...)
	cmd.Dir = workspaceRoot()
	cmd.Env = append(os.Environ(), config.Env...)
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("mcp stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("mcp stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("start mcp server: %w", err)
	}
	c := &Client{config: config, cmd: cmd, stdin: stdin, output: bufio.NewReader(stdout), stderr: stderr}
	if _, err := c.request(ctx, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]string{"name": "kratos-demo", "version": "0.1.0"},
	}); err != nil {
		c.Close()
		return nil, nil, err
	}
	if err := c.notify("notifications/initialized", map[string]any{}); err != nil {
		c.Close()
		return nil, nil, err
	}
	raw, err := c.request(ctx, "tools/list", map[string]any{})
	if err != nil {
		c.Close()
		return nil, nil, err
	}
	tools, err := decodeToolsList(raw)
	if err != nil {
		c.Close()
		return nil, nil, err
	}
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

func (c *Client) Call(ctx context.Context, name, input string) (string, error) {
	args := map[string]any{}
	input = strings.TrimSpace(input)
	if input != "" {
		if err := json.Unmarshal([]byte(input), &args); err != nil {
			args["query"] = input
		}
	}
	raw, err := c.request(ctx, "tools/call", map[string]any{"name": name, "arguments": args})
	if err != nil {
		return "", err
	}
	return decodeToolCall(raw)
}

func (c *Client) Close() {
	if c == nil {
		return
	}
	_ = c.stdin.Close()
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	if c.cmd != nil {
		_ = c.cmd.Wait()
	}
}

func (c *Client) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, c.config.Timeout)
	defer cancel()
	c.nextID++
	id := c.nextID
	if err := c.write(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		return nil, err
	}
	type response struct {
		ID     int64           `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	result := make(chan struct {
		raw json.RawMessage
		err error
	}, 1)
	go func() {
		for {
			line, err := c.output.ReadBytes('\n')
			if err != nil {
				if c.stderr != nil && c.stderr.Len() > 0 {
					err = fmt.Errorf("%w: %s", err, strings.TrimSpace(c.stderr.String()))
				}
				result <- struct {
					raw json.RawMessage
					err error
				}{err: err}
				return
			}
			var r response
			if err := json.Unmarshal(line, &r); err != nil {
				continue
			}
			if r.ID != id {
				continue
			}
			if r.Error != nil {
				result <- struct {
					raw json.RawMessage
					err error
				}{err: errors.New(r.Error.Message)}
				return
			}
			result <- struct {
				raw json.RawMessage
				err error
			}{raw: r.Result}
			return
		}
	}()
	select {
	case r := <-result:
		if r.err != nil {
			return nil, fmt.Errorf("mcp %s: %w", method, r.err)
		}
		return r.raw, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("mcp %s: %w", method, ctx.Err())
	}
}

func (c *Client) notify(method string, params any) error {
	return c.write(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}
func (c *Client) write(payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = c.stdin.Write(append(raw, '\n'))
	return err
}

func decodeToolsList(raw []byte) ([]lmm.Tool, error) {
	var response struct {
		Tools []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			InputSchema any    `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, fmt.Errorf("decode mcp tools/list: %w", err)
	}
	tools := make([]lmm.Tool, 0, len(response.Tools))
	for _, tool := range response.Tools {
		if strings.TrimSpace(tool.Name) == "" || tool.InputSchema == nil {
			return nil, errors.New("mcp tool missing name or inputSchema")
		}
		tools = append(tools, lmm.Tool{Type: "function", Function: &lmm.FunctionDefinition{Name: tool.Name, Description: tool.Description, Parameters: tool.InputSchema}})
	}
	return tools, nil
}

func decodeToolCall(raw []byte) (string, error) {
	var response struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return "", fmt.Errorf("decode mcp tools/call: %w", err)
	}
	parts := make([]string, 0, len(response.Content))
	for _, part := range response.Content {
		if part.Type == "text" {
			parts = append(parts, part.Text)
		}
	}
	output := strings.Join(parts, "\n")
	if response.IsError {
		return output, errors.New(output)
	}
	return output, nil
}
