# system-info-provider

A small Go daemon that streams system, workspace, and Bluetooth state as JSON. It can print to stdout or act as a Unix socket server for multiple subscribers.

## Features
- Periodic system stats: time, CPU, memory, battery, and network
- Workspace updates for Hyprland or Mango WC
- Bluetooth adapter + device state via BlueZ (D-Bus)
- Output as JSON to stdout or over a Unix socket

## Requirements
- Linux (uses `/sys`, BlueZ, and Wayland compositor IPC)
- Go 1.25+
- For workspace data:
  - Hyprland (uses `HYPRLAND_INSTANCE_SIGNATURE` + IPC sockets), or
  - Mango WC (requires `mmsg` in PATH)

## Build
```bash
go build -o system-info-provider
```

## Installation

### Nix (flake)
Build and run directly with flakes:
```bash
nix build
./result/bin/system-info-provider system
```

Or run without building an output path:
```bash
nix run . -- system
```

## Usage
The program expects a single argument that selects the data stream.

```bash
./system-info-provider <data_type>
```

Supported `data_type` values:
- `system` — periodic system stats
- `workspace` — compositor workspace state (Hyprland or Mango)
- `hyprland` — legacy alias for `workspace`
- `bluetooth` — BlueZ adapter + device state
- `socket` — start the Unix socket server and broadcast all streams

### Examples
Stream system info to stdout:
```bash
./system-info-provider system
```

Stream workspace changes:
```bash
./system-info-provider workspace
```

Start socket server:
```bash
./system-info-provider socket
```

The socket server listens on:
```
/tmp/system-info-provider.sock
```

Clients can subscribe by sending lines like:
```
SUB SYSTEM
SUB WORKSPACE
SUB BLUETOOTH
```

## Output format
All messages are JSON objects of the form:
```json
{"type":"<data_type>","data":{...}}
```

Example `system` payload (abridged):
```json
{
  "type": "system",
  "data": {
    "time": "Mon 01 Jan 15:04:05",
    "cpu_per_core": [4.3, 9.1],
    "cpu_average": 6.7,
    "memory_used": 123456789,
    "memory_total": 17179869184,
    "battery": {"percentage": 82, "state": "Discharging"},
    "network": {"interface": "wlan0", "ip_address": "192.168.1.20"}
  }
}
```

Workspace payload:
```json
{
  "type": "workspace",
  "data": {"current_workspace": 2, "workspace_list": [1, 2, 3]}
}
```

Bluetooth payload:
```json
{
  "type": "bluetooth",
  "data": {"powered": true, "devices": {"/org/bluez/hci0/dev_XX": {"name": "Device"}}}
}
```

## Notes
- Battery info is read from `/sys/class/power_supply/BAT0/uevent`.
- Network info uses the first active interface with an IPv4 address.
- In `socket` mode, clients receive an initial state snapshot on subscribe.

## Systemd
Example user service to run the socket server:
```ini
[Unit]
Description=System Info Provider
After=network.target

[Service]
ExecStart=%h/bin/system-info-provider socket
Restart=on-failure
RestartSec=2

[Install]
WantedBy=default.target
```

If you installed the binary elsewhere, adjust `ExecStart` to the correct path.
