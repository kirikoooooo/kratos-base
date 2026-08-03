package langfuse

import "testing"

func TestMetadataAttributesKeepTypesAndRejectInvalidKeys(t *testing.T) {
	attrs := metadataAttributes(map[string]any{
		"name":    "agent",
		"enabled": true,
		"retries": 2,
		"ratio":   0.5,
		"details": map[string]any{"mode": "remote"},
		"bad key": "ignored",
	})
	got := make(map[string]string, len(attrs))
	for _, attr := range attrs {
		got[string(attr.Key)] = attr.Value.Emit()
	}
	if got["langfuse.trace.metadata.name"] != "agent" || got["langfuse.trace.metadata.enabled"] != "true" || got["langfuse.trace.metadata.retries"] != "2" || got["langfuse.trace.metadata.ratio"] != "0.5" {
		t.Fatalf("unexpected scalar attributes: %#v", got)
	}
	if got["langfuse.trace.metadata.details"] != `{"mode":"remote"}` {
		t.Fatalf("details = %q", got["langfuse.trace.metadata.details"])
	}
	if _, ok := got["langfuse.trace.metadata.bad key"]; ok {
		t.Fatal("invalid metadata key was exported")
	}
}
