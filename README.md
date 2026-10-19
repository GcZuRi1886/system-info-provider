# system-info-provider

A small Go daemon that streams system, workspace, Bluetooth, brightness, and media playback state as JSON. It can print to stdout or act as a Unix socket server for multiple subscribers.

## Features
- Periodic system stats: time, CPU, memory, battery, and network (including Wi-Fi SSID, signal, speeds, and VPN)
- Workspace updates for Hyprland or Mango WC
- Bluetooth adapter + device state via BlueZ (D-Bus)
- Currently playing media via MPRIS (D-Bus session bus)
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
- `workspace` — compositor workspace state (Hyprland or Mango WC)
- `hyprland` — legacy alias for `workspace`
- `bluetooth` — BlueZ adapter + device state
- `brightness` — current screen brightness level
- `media` — currently playing media (MPRIS)
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
$XDG_RUNTIME_DIR/system-info-provider.sock
```

Typically this resolves to `/run/user/$UID/system-info-provider.sock`.

Clients can subscribe by sending lines like:
```
SUB SYSTEM
SUB WORKSPACE
SUB BLUETOOTH
SUB BRIGHTNESS
SUB MEDIA
```

Upon subscribing, the client receives an initial state snapshot immediately, then live updates as they occur.

Clients can also send commands with `CMD <TYPE> <action> [arg]`. Currently only `MEDIA` accepts commands; they act on the same player that the `media` stream reports:
```
CMD MEDIA play-pause
CMD MEDIA play
CMD MEDIA pause
CMD MEDIA next
CMD MEDIA previous
CMD MEDIA seek 42.5
```

`seek` takes an absolute position in seconds. The server replies with `OK media <action>` on success or `ERROR <reason>` (e.g. `ERROR no active player`, or when the player's matching `can_*` flag is `false`). Commands don't produce a direct state message; the resulting change arrives through the normal `media` subscription.

## Output format
All messages are JSON objects of the form:
```json
{"type":"<data_type>","data":{...}}
```

In stdout mode, each update overwrites the previous line (carriage return). In socket mode, each update is newline-delimited.

### `system` payload
```json
{
  "type": "system",
  "data": {
    "time": "Mon 01 Jan 15:04:05",
    "cpu_per_core": [4.3, 9.1, 2.0, 7.5],
    "cpu_average": 5.7,
    "memory_used": 8589934592,
    "memory_total": 17179869184,
    "battery": {
      "percentage": 82,
      "state": "Discharging",
      "time_to_empty": 3.5,
      "time_to_full": 0
    },
    "network": {
      "interface": "wlan0",
      "ip_address": "192.168.1.20",
      "is_wifi": true,
      "ssid": "MyNetwork",
      "is_connected": true,
      "signal_strength": 74,
      "down_speed_kbps": 1024.5,
      "up_speed_kbps": 128.3,
      "bytes_sent": 104857600,
      "bytes_recv": 524288000,
      "vpn": {
        "is_connected": true,
        "name": "wg0",
        "interface": "wg0",
        "type": "wireguard"
      }
    }
  }
}
```

### `workspace` payload
```json
{
  "type": "workspace",
  "data": {
    "current_workspace": 2,
    "workspace_list": [1, 2, 3],
    "focused_monitor": "DP-1"
  }
}
```

### `bluetooth` payload
```json
{
  "type": "bluetooth",
  "data": {
    "powered": true,
    "devices": {
      "/org/bluez/hci0/dev_AA_BB_CC_DD_EE_FF": {
        "name": "My Headphones",
        "address": "AA:BB:CC:DD:EE:FF",
        "connected": true,
        "paired": true,
        "trusted": true,
        "adapter": "/org/bluez/hci0"
      }
    }
  }
}
```

### `brightness` payload
```json
{
  "type": "brightness",
  "data": {
    "current": 800,
    "maximum": 1200,
    "percentage": 0.667
  }
}
```

### `media` payload
```json
{
  "type": "media",
  "data": {
    "active": true,
    "player": "spotify",
    "status": "Playing",
    "title": "Song Title",
    "artist": "Artist A, Artist B",
    "album": "Album Name",
    "art_url": "https://i.scdn.co/image/...",
    "length": 215.4,
    "position": 42.1,
    "position_updated": 1792419316539,
    "can_go_next": true,
    "can_go_previous": true,
    "can_play": true,
    "can_pause": true,
    "can_seek": true
  }
}
```

When no player is running, `active` is `false` and the player fields are omitted.

`length` and `position` are in seconds. Players do not signal position continuously, so `position` is only refreshed on play/pause, track changes, and seeks. `position_updated` is the Unix timestamp (ms) at which it was read; while `status` is `Playing`, clients can compute the current position as `position + (now - position_updated) / 1000`.

## Notes
- Battery info is read from `/sys/class/power_supply/BAT0/uevent`.
- Network info uses the first active interface with an IPv4 address.
- VPN detection covers WireGuard, OpenVPN, ProtonVPN, NordVPN, Mullvad, and PPTP (by interface name pattern and `/sys` inspection).
- In `socket` mode, clients receive an initial state snapshot on subscribe.
- With several MPRIS players, the one that is playing is reported; otherwise the most recently active one. `playerctld` is ignored since it only proxies other players.

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
