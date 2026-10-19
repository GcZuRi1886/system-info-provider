package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

func getSocketPath() string {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = fmt.Sprintf("/run/user/%d", os.Getuid())
	}
	return filepath.Join(runtimeDir, "system-info-provider.sock")
}

var socketPath = getSocketPath()

// ----- emitToConsole updates to stdout -----
func emitToConsole(dataType string, data any) {
	dataJSON, _ := json.Marshal(data)
	fmt.Printf("\r%s", string(dataJSON))
}

// listenWorkspaceEvents starts the appropriate workspace provider based on detected compositor
func listenWorkspaceEvents(emit func(dataType string, data any)) {
	provider := NewWorkspaceProvider()
	if provider == nil {
		log.Println("No supported compositor detected. Workspace events disabled.")
		return
	}
	log.Printf("Detected compositor: %s", provider.Name())
	provider.Listen(emit)
}

// runLogsCommand handles `logs [--follow|-f]`
func runLogsCommand(args []string) {
	follow := false
	for _, a := range args {
		switch a {
		case "--follow", "-f":
			follow = true
		default:
			fmt.Fprintf(os.Stderr, "Unknown logs option: %s\n", a)
			os.Exit(1)
		}
	}
	if err := printLogs(follow); err != nil {
		fmt.Fprintf(os.Stderr, "Could not read logs at %s: %v\n", logPath, err)
		os.Exit(1)
	}
}

// ----- main -----
func main() {
	args := os.Args
	if len(args) >= 2 && args[1] == "logs" {
		runLogsCommand(args[2:])
		return
	}

	if len(args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <data_type>\n", args[0])
		fmt.Fprintf(os.Stderr, "       %s logs [--follow|-f]\n", args[0])
		fmt.Fprintln(os.Stderr, "data_type can be: workspace, system, bluetooth, brightness, media, socket")
		os.Exit(1)
	}
	requestedData := args[1]
	setupLogging(requestedData)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch requestedData {
	case "workspace":
		go listenWorkspaceEvents(emitToConsole)
	case "hyprland":
		// Legacy: still supported for backwards compatibility
		go listenWorkspaceEvents(emitToConsole)
	case "system":
		go sysInfoLoop(emitToConsole)
	case "bluetooth":
		go listenForBluetoothChanges(emitToConsole)
	case "brightness":
		go listenForBrightnessChanges(emitToConsole)
	case "media":
		go listenForMediaChanges(emitToConsole)
	case "socket":
		_, err := connectToSocket(socketPath)
		if err != nil {
			log.Fatalf("Failed to connect to socket: %v", err)
		}
		go sysInfoLoop(broadcast)
		go listenWorkspaceEvents(broadcast)
		go listenForBluetoothChanges(broadcast)
		go listenForBrightnessChanges(broadcast)
		go listenForMediaChanges(broadcast)
	default:
		log.Fatalf("Unknown requested data type: %s", requestedData)
	}
	log.Println("Daemon started. Press Ctrl+C to exit.")
	<-ctx.Done()
	log.Println("Shutting down daemon.")
}
