package toolcatalog

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"sync"

	lmm "kratos-demo/internal/biz/llm"
)

//go:embed *.json
var embeddedDefinitions embed.FS

var (
	defaultCatalogOnce sync.Once
	defaultCatalog     *Catalog
	defaultCatalogErr  error
)

type Definition struct {
	Type     string             `json:"type"`
	Function FunctionDefinition `json:"function"`
}

type FunctionDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters,omitempty"`
	Strict      bool   `json:"strict,omitempty"`
}

type Handler func(ctx context.Context, input string) (string, error)

type BindingSpec struct {
	Name    string
	Handler Handler
}

type Catalog struct {
	definitions map[string]Definition
}

func DefaultCatalog() (*Catalog, error) {
	defaultCatalogOnce.Do(func() {
		defaultCatalog, defaultCatalogErr = LoadCatalog()
	})
	return defaultCatalog, defaultCatalogErr
}

func LoadCatalog() (*Catalog, error) {
	entries, err := fs.ReadDir(embeddedDefinitions, ".")
	if err != nil {
		return nil, fmt.Errorf("read embedded tool definitions failed: %w", err)
	}

	definitions := make(map[string]Definition, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}

		raw, err := embeddedDefinitions.ReadFile(entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read tool definition %s failed: %w", entry.Name(), err)
		}

		var definition Definition
		if err := json.Unmarshal(raw, &definition); err != nil {
			return nil, fmt.Errorf("unmarshal tool definition %s failed: %w", entry.Name(), err)
		}
		if err := validateDefinition(definition, entry.Name()); err != nil {
			return nil, err
		}

		name := strings.TrimSpace(definition.Function.Name)
		if _, exists := definitions[name]; exists {
			return nil, fmt.Errorf("duplicate tool definition: %s", name)
		}
		definitions[name] = definition
	}

	return &Catalog{definitions: definitions}, nil
}

func (c *Catalog) Definitions() []Definition {
	if c == nil || len(c.definitions) == 0 {
		return nil
	}

	names := make([]string, 0, len(c.definitions))
	for name := range c.definitions {
		names = append(names, name)
	}
	sort.Strings(names)

	definitions := make([]Definition, 0, len(names))
	for _, name := range names {
		definitions = append(definitions, c.definitions[name])
	}
	return definitions
}

func (c *Catalog) Lookup(name string) (Definition, bool) {
	if c == nil {
		return Definition{}, false
	}
	definition, ok := c.definitions[strings.TrimSpace(name)]
	return definition, ok
}

func (c *Catalog) AsLLMTools(names []string) ([]lmm.Tool, error) {
	tools := make([]lmm.Tool, 0, len(names))
	for _, name := range names {
		definition, ok := c.Lookup(name)
		if !ok {
			return nil, fmt.Errorf("tool definition not found: %s", strings.TrimSpace(name))
		}
		tools = append(tools, lmm.Tool{
			Type: definition.Type,
			Function: &lmm.FunctionDefinition{
				Name:        definition.Function.Name,
				Description: definition.Function.Description,
				Parameters:  definition.Function.Parameters,
				Strict:      definition.Function.Strict,
			},
		})
	}
	return tools, nil
}

func validateDefinition(definition Definition, source string) error {
	if strings.TrimSpace(definition.Type) != "function" {
		return fmt.Errorf("tool definition %s type must be function", source)
	}
	if strings.TrimSpace(definition.Function.Name) == "" {
		return fmt.Errorf("tool definition %s function.name is empty", source)
	}
	if strings.TrimSpace(definition.Function.Description) == "" {
		return fmt.Errorf("tool definition %s function.description is empty", source)
	}
	if definition.Function.Parameters == nil {
		return fmt.Errorf("tool definition %s function.parameters is empty", source)
	}
	return nil
}
