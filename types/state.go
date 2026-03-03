// Package types defines types for this project
package types

type CurrentStateData struct {
	Time        string      `json:"time"`
	CPUPerCore  []float64   `json:"cpu_per_core"`
	CPUAverage  float64     `json:"cpu_average"`
	MemoryUsed  int         `json:"memory_used"`
	MemoryTotal int         `json:"memory_total"`
	Battery     BatteryInfo `json:"battery"`
	Network     NetworkInfo `json:"network"`
}

type BatteryInfo struct {
	Percentage  int     `json:"percentage"`
	State       string  `json:"state"`
	TimeToEmpty float64 `json:"time_to_empty"`
	TimeToFull  float64 `json:"time_to_full"`
}

type NetworkInfo struct {
	Interface      string  `json:"interface"`
	IPAddress      string  `json:"ip_address"`
	IsWifi         bool    `json:"is_wifi"`
	SSID           string  `json:"ssid,omitempty"`
	IsConnected    bool    `json:"is_connected"`
	SignalStrength int     `json:"signal_strength"` // in percentage
	DownSpeed      float64 `json:"down_speed_kbps"`
	UpSpeed        float64 `json:"up_speed_kbps"`
	BytesSent      uint64  `json:"bytes_sent"`
	BytesRecv      uint64  `json:"bytes_recv"`
	VPN            VPNInfo `json:"vpn"`
}

type VPNInfo struct {
	IsConnected bool   `json:"is_connected"`
	Name        string `json:"name,omitempty"`
	Interface   string `json:"interface,omitempty"`
	Type        string `json:"type,omitempty"` // wireguard, openvpn, etc.
}

// AudioInfo holds volume and mute state
type AudioInfo struct {
	Volume float64 `json:"volume"` // 0.0–1.0
	Muted  bool    `json:"muted"`
	Name   string  `json:"name,omitempty"`
}
