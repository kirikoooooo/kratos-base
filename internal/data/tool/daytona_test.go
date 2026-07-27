package tool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kratos-demo/internal/conf"
	toolcatalog "kratos-demo/third_party/tools"
)

func TestDaytonaToolIsOnlyExposedWhenEnabled(t *testing.T) {
	tools, err := NewToolExecutor(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if hasToolBinding(tools.Bindings(), "daytona_data_analysis") {
		t.Fatal("Daytona tool must not be exposed while disabled")
	}
	tools.SetDaytona(&conf.Runtime_Daytona{Enabled: true})
	if !hasToolBinding(tools.Bindings(), "daytona_data_analysis") {
		t.Fatal("Daytona tool must be exposed when enabled")
	}
}

func TestDaytonaDataAnalysisIntegration(t *testing.T) {
	if os.Getenv("KRATOS_RUN_DAYTONA_INTEGRATION") != "1" {
		t.Skip("set KRATOS_RUN_DAYTONA_INTEGRATION=1 to create a real Daytona sandbox")
	}
	if os.Getenv("DAYTONA_API_KEY") == "" {
		t.Fatal("DAYTONA_API_KEY is required")
	}
	tools, err := NewToolExecutor(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	tools.SetDaytona(&conf.Runtime_Daytona{
		Enabled:           true,
		ApiKeyEnv:         "DAYTONA_API_KEY",
		ApiUrl:            "https://app.daytona.io/api",
		Snapshot:          "daytona-small",
		AutoDeleteMinutes: 0,
	})
	input := `{"code":"import matplotlib.pyplot as plt\nplt.bar(['A', 'B', 'C'], [3, 5, 2])\nplt.title('Daytona smoke test')\nplt.savefig('/tmp/daytona-smoke.png')\nprint('chart created')", "output_path":"/tmp/daytona-smoke.png"}`
	output, err := tools.daytonaDataAnalysis(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "exit_code: 0") {
		t.Fatalf("unexpected Daytona output: %s", output)
	}
	chart := filepath.Join(tools.root, ".myagent", "artifacts", "adhoc", "daytona-smoke.png")
	info, err := os.Stat(chart)
	if err != nil || info.Size() == 0 {
		t.Fatalf("chart artifact is missing or empty: %s (%v)", chart, err)
	}
}

func hasToolBinding(bindings []toolcatalog.BindingSpec, name string) bool {
	for _, binding := range bindings {
		if binding.Name == name {
			return true
		}
	}
	return false
}
