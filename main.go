package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
)

var socketPath = "/tmp/system-info-provider.sock"

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

// ----- main -----
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	args := os.Args
	if len(args) != 2 {
		log.Fatalf("Usage: %s <data_type>", args[0])
	}
	requestedData := args[1]

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
	case "socket":
		_, err := connectToSocket(socketPath)
		if err != nil {
			log.Fatalf("Failed to connect to socket: %v", err)
		}
		go sysInfoLoop(broadcast)
		go listenWorkspaceEvents(broadcast)
		go listenForBluetoothChanges(broadcast)
	default:
		log.Fatalf("Unknown requested data type: %s", requestedData)
	}
	log.Println("Daemon started. Press Ctrl+C to exit.")
	<-ctx.Done()
	log.Println("Shutting down daemon.")
}
