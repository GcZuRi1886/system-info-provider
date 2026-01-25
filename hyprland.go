package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/GcZuRi1886/system-info-provider/types"
)

// HyprlandProvider implements WorkspaceProvider for Hyprland
type HyprlandProvider struct{}

// NewHyprlandProvider creates a new Hyprland workspace provider
func NewHyprlandProvider() *HyprlandProvider {
	return &HyprlandProvider{}
}

// Name returns the compositor name
func (h *HyprlandProvider) Name() string {
	return "hyprland"
}

func (h *HyprlandProvider) openSocket(sockName string) (net.Conn, error) {
	sig := os.Getenv("HYPRLAND_INSTANCE_SIGNATURE")
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	sock := filepath.Join(runtimeDir, "hypr", sig, sockName)
	addr := net.UnixAddr{Name: sock, Net: "unix"}

	conn, err := net.DialUnix("unix", nil, &addr)
	if err != nil {
		return nil, fmt.Errorf("cannot open Hyprland socket %s: %v", sockName, err)
	}
	return conn, nil
}

func (h *HyprlandProvider) openCommandSocket() (net.Conn, error) {
	return h.openSocket(".socket.sock")
}

func (h *HyprlandProvider) sendCommand(cmd string) ([]byte, error) {
	conn, err := h.openCommandSocket()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	_, err = conn.Write([]byte(cmd))
	if err != nil {
		return nil, err
	}

	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}

	return buf[:n], nil
}

// GetWorkspaceState retrieves the current workspace state from Hyprland
func (h *HyprlandProvider) GetWorkspaceState() (*types.WorkspaceInfo, error) {
	out, err := h.sendCommand("j/monitors")
	if err != nil {
		return nil, fmt.Errorf("error getting monitors: %v", err)
	}
	current := h.parseWorkspaceCurrent(out)

	out2, err := h.sendCommand("j/workspaces")
	if err != nil {
		return nil, fmt.Errorf("error getting workspaces: %v", err)
	}
	ids := h.parseWorkspaceIDs(out2)

	return &types.WorkspaceInfo{
		Current: current,
		List:    ids,
	}, nil
}

func (h *HyprlandProvider) parseWorkspaceIDs(workspacesJSON []byte) []int {
	var workspaces []types.Workspace

	if err := json.Unmarshal(workspacesJSON, &workspaces); err != nil {
		return nil
	}

	var ids []int
	for _, ws := range workspaces {
		ids = append(ids, ws.ID)
	}
	slices.Sort(ids)
	return ids
}

func (h *HyprlandProvider) parseWorkspaceCurrent(monitorsJSON []byte) int {
	var monitors []types.Monitor
	if err := json.Unmarshal(monitorsJSON, &monitors); err != nil {
		return 0
	}

	if len(monitors) == 0 {
		return 0
	}

	return monitors[0].ActiveWorkspace.ID
}

// Listen starts listening for workspace events from Hyprland
func (h *HyprlandProvider) Listen(emit func(dataType string, data any)) {
	wrapper := types.Wrapper{
		Type: "workspace",
	}

	// Get initial state
	state, err := h.GetWorkspaceState()
	if err != nil {
		log.Printf("Error getting initial Hyprland workspace state: %v", err)
	} else {
		wrapper.Data = state
		emit(wrapper.Type, wrapper)
	}

	f, err := h.openSocket(".socket2.sock")
	if err != nil {
		log.Printf("Error opening Hyprland event socket: %v", err)
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "workspace>>") ||
			strings.HasPrefix(line, "createworkspace>>") ||
			strings.HasPrefix(line, "destroyworkspace>>") {
			state, err := h.GetWorkspaceState()
			if err != nil {
				log.Printf("Error getting Hyprland workspace state: %v", err)
				continue
			}
			wrapper.Data = state
			emit(wrapper.Type, wrapper)
		}
	}
}

// Legacy function for backwards compatibility - wraps the provider
func listenHyprlandEventSocket(emit func(dataType string, data any)) {
	provider := NewHyprlandProvider()
	provider.Listen(emit)
}
