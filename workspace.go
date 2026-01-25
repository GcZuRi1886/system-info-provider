package main

import (
	"github.com/GcZuRi1886/system-info-provider/types"
)

// WorkspaceProvider is an interface for compositor-specific workspace implementations
type WorkspaceProvider interface {
	// GetWorkspaceState retrieves the current workspace state
	GetWorkspaceState() (*types.WorkspaceInfo, error)
	// Listen starts listening for workspace events and calls emit on changes
	Listen(emit func(dataType string, data any))
	// Name returns the compositor name
	Name() string
}

// NewWorkspaceProvider creates a workspace provider for the detected compositor
func NewWorkspaceProvider() WorkspaceProvider {
	compositor := DetectCompositor()
	switch compositor {
	case CompositorHyprland:
		return NewHyprlandProvider()
	case CompositorMango:
		return NewMangoProvider()
	default:
		return nil
	}
}
