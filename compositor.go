package main

import (
	"os"
	"os/exec"
)

type Compositor int

const (
	CompositorUnknown Compositor = iota
	CompositorHyprland
	CompositorMango
)

func (c Compositor) String() string {
	switch c {
	case CompositorHyprland:
		return "hyprland"
	case CompositorMango:
		return "mango"
	default:
		return "unknown"
	}
}

// DetectCompositor checks environment variables and available tools to determine
// which Wayland compositor is currently running
func DetectCompositor() Compositor {
	// Check for Hyprland first via its instance signature
	if sig := os.Getenv("HYPRLAND_INSTANCE_SIGNATURE"); sig != "" {
		return CompositorHyprland
	}

	// Check for Mango WC by looking for the mmsg tool
	if _, err := exec.LookPath("mmsg"); err == nil {
		// Verify mmsg is responsive (mango is running)
		cmd := exec.Command("mmsg", "-g", "-t")
		if err := cmd.Run(); err == nil {
			return CompositorMango
		}
	}

	return CompositorUnknown
}
