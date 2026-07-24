package tool

import (
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

func hasToolBinding(bindings []toolcatalog.BindingSpec, name string) bool {
	for _, binding := range bindings {
		if binding.Name == name {
			return true
		}
	}
	return false
}
