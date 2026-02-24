package main

import (
	"bufio"
	"log"
	"os/exec"
	"strconv"
	"strings"

	"github.com/GcZuRi1886/system-info-provider/types"
)

// MangoProvider implements WorkspaceProvider for Mango WC
type MangoProvider struct{}

// NewMangoProvider creates a new Mango WC workspace provider
func NewMangoProvider() *MangoProvider {
	return &MangoProvider{}
}

// Name returns the compositor name
func (m *MangoProvider) Name() string {
	return "mango"
}

// GetWorkspaceState retrieves the current workspace state from Mango WC
func (m *MangoProvider) GetWorkspaceState() (*types.WorkspaceInfo, error) {
	// Run mmsg -g -t to get tag information
	cmd := exec.Command("mmsg", "-g", "-t")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	return m.parseTagOutput(string(output))
}

// parseTagOutput parses the output from mmsg -g -t
// Format observed:
//   eDP-1 tag 1 1 1 1      <- per-tag: monitor, "tag", tag_num, state, has_clients, is_focused
//   eDP-1 tag 2 0 1 0
//   ...
//   eDP-1 clients 2        <- client count
//   eDP-1 tags 3 2 0       <- decimal masks: monitor, "tags", occupied, active, urgent
//   eDP-1 tags 000000011 000000010 000000000  <- binary masks
//
// We parse the decimal masks line: "tags <occupied> <active> <urgent>"
func (m *MangoProvider) parseTagOutput(output string) (*types.WorkspaceInfo, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")

	info := &types.WorkspaceInfo{
		Current:        1,
		List:           []int{},
		FocusedMonitor: "",
	}

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		// Look for the decimal masks line: "<monitor> tags <occupied> <active> <urgent>"
		// The decimal version has short numeric strings (not 9-char binary)
		if fields[1] == "tags" && len(fields) >= 4 {
			// Check if this is the decimal line (not binary)
			// Binary masks are 9 characters like "000000011"
			if len(fields[2]) <= 4 {
				occupiedMask, err1 := strconv.Atoi(fields[2])
				activeMask, err2 := strconv.Atoi(fields[3])

				if err1 == nil && err2 == nil {
					// The monitor name is the first field
					info.FocusedMonitor = fields[0]

					// Parse occupied tags from bitmask
					for i := 0; i < 32; i++ {
						if occupiedMask&(1<<i) != 0 {
							info.List = append(info.List, i+1) // Tags are 1-indexed
						}
					}

					// Find current (active) tag from active mask
					for i := 0; i < 32; i++ {
						if activeMask&(1<<i) != 0 {
							info.Current = i + 1 // Tags are 1-indexed
							break
						}
					}
					break // Found the decimal mask line, we're done
				}
			}
		}
	}

	// If no occupied tags found, at least include the current tag
	if len(info.List) == 0 {
		info.List = append(info.List, info.Current)
	}

	return info, nil
}

// Listen starts listening for workspace events from Mango WC
func (m *MangoProvider) Listen(emit func(dataType string, data any)) {
	wrapper := types.Wrapper{
		Type: "workspace",
	}

	// Track last state to avoid duplicate emissions
	var lastCurrent int
	var lastListStr string

	emitIfChanged := func(state *types.WorkspaceInfo) {
		// Build a simple string representation of the list for comparison
		listStr := ""
		for _, id := range state.List {
			listStr += strconv.Itoa(id) + ","
		}

		if state.Current != lastCurrent || listStr != lastListStr {
			lastCurrent = state.Current
			lastListStr = listStr
			wrapper.Data = state
			emit(wrapper.Type, wrapper)
		}
	}

	// Get initial state
	state, err := m.GetWorkspaceState()
	if err != nil {
		log.Printf("Error getting initial Mango workspace state: %v", err)
	} else {
		emitIfChanged(state)
	}

	// Start watching for changes with mmsg -w -t
	cmd := exec.Command("mmsg", "-w", "-t")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("Error creating Mango watch pipe: %v", err)
		return
	}

	if err := cmd.Start(); err != nil {
		log.Printf("Error starting Mango watch: %v", err)
		return
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		// On any output, refresh the workspace state
		state, err := m.GetWorkspaceState()
		if err != nil {
			log.Printf("Error getting Mango workspace state: %v", err)
			continue
		}
		emitIfChanged(state)
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Error reading Mango watch output: %v", err)
	}
}
