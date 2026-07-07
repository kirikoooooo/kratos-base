package tool

// ToolRepository is the port for accessing registered tool bindings.
// Implementations live in the data layer.
type ToolRepository interface {
	// Bindings returns the set of tool name → handler bindings
	// available to the agent runtime.
	Bindings() []BindingSpec
}
