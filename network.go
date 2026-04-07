package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/GcZuRi1886/system-info-provider/types"
	"github.com/mdlayher/wifi"
	psnet "github.com/shirou/gopsutil/v4/net"
)

// getAllActiveInterfaces returns a list of active (up, non-loopback) interfaces
func getAllActiveInterfaces() ([]net.Interface, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var active []net.Interface
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		active = append(active, iface)
	}
	return active, nil
}

// getIPAddress returns first IPv4 for given interface
func getIPAddress(iface net.Interface) string {
	addrs, err := iface.Addrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		ip := strings.Split(addr.String(), "/")[0]
		if strings.Contains(ip, ".") {
			return ip
		}
	}
	return ""
}

func isWifiInterface(iface string) bool {
	_, err := os.Stat(fmt.Sprintf("/sys/class/net/%s/wireless", iface))
	return err == nil
}

func getWifiDetails(ifaceName string) (string, int, error) {
	c, err := wifi.New()
	if err != nil {
		return "", 0, fmt.Errorf("wifi init: %w", err)
	}
	defer c.Close()

	ifaces, err := c.Interfaces()
	if err != nil {
		return "", 0, fmt.Errorf("list wifi interfaces: %w", err)
	}

	var target *wifi.Interface
	for _, ifi := range ifaces {
		if ifi.Name == ifaceName {
			target = ifi
			break
		}
	}
	if target == nil {
		return "", 0, fmt.Errorf("no wifi interface found for %s", ifaceName)
	}

	bss, err := c.BSS(target)
	if err != nil {
		return "", 0, fmt.Errorf("get BSS: %w", err)
	}

	stas, err := c.StationInfo(target)
	if err != nil {
		return "", 0, fmt.Errorf("get station info: %w", err)
	}

	signalDBm := stas[0].Signal

	signalPercent := dbmToPercent(signalDBm)

	return bss.SSID, signalPercent, nil
}

func dbmToPercent(dbm int) int {
	// Rough linear scale from -100 (0%) to -50 (100%)
	if dbm <= -100 {
		return 0
	} else if dbm >= -50 {
		return 100
	}
	return 2 * (dbm + 100)
}

// isVirtualInterface returns true for virtual/container network interfaces that
// should not be considered as a primary network connection (e.g. Docker/Podman
// bridges, veth pairs, dummy interfaces, etc.).
func isVirtualInterface(name string) bool {
	virtualPrefixes := []string{
		"br-",    // Docker/Podman named bridge networks
		"docker", // docker0 default bridge
		"podman", // podman0 default bridge
		"veth",   // virtual ethernet pairs (container side)
		"virbr",  // libvirt/KVM bridges
		"vnet",   // libvirt virtual NICs
		"dummy",  // dummy interfaces
		"lxcbr",  // LXC bridges
		"lxdbr",  // LXD bridges
		"vxlan",  // VXLAN overlay interfaces
	}
	for _, prefix := range virtualPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// isVPNInterface checks if an interface is a VPN interface based on its name and type
func isVPNInterface(ifaceName string) (bool, string) {
	// Check for common VPN interface name patterns
	vpnPatterns := map[string]string{
		"tun":       "openvpn",
		"tap":       "openvpn",
		"wg":        "wireguard",
		"tailscale": "tailscale",
		"proton":    "protonvpn",
		"nordlynx":  "nordvpn",
		"mullvad":   "mullvad",
		"ppp":       "pptp",
	}

	for prefix, vpnType := range vpnPatterns {
		if strings.HasPrefix(ifaceName, prefix) {
			return true, vpnType
		}
	}

	// Check if it's a WireGuard interface by looking for the wireguard sysfs directory
	wgPath := filepath.Join("/sys/class/net", ifaceName, "uevent")
	if data, err := os.ReadFile(wgPath); err == nil {
		if strings.Contains(string(data), "wireguard") {
			return true, "wireguard"
		}
	}

	// Check for TUN/TAP device type
	tunFlagsPath := filepath.Join("/sys/class/net", ifaceName, "tun_flags")
	if _, err := os.Stat(tunFlagsPath); err == nil {
		return true, "tun"
	}

	return false, ""
}

// hasRoutableIP checks if an interface has a routable (non-link-local) IP address
func hasRoutableIP(iface net.Interface) bool {
	addrs, err := iface.Addrs()
	if err != nil {
		return false
	}

	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}

		ip := ipNet.IP
		// Skip loopback and link-local addresses
		if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			continue
		}

		// Found a routable IP (either IPv4 or IPv6)
		return true
	}

	return false
}

// getVPNInfo scans all active interfaces and returns VPN connection info
func getVPNInfo() types.VPNInfo {
	interfaces, err := getAllActiveInterfaces()
	if err != nil {
		return types.VPNInfo{IsConnected: false}
	}

	for _, iface := range interfaces {
		if isVPN, vpnType := isVPNInterface(iface.Name); isVPN {
			// Check if the VPN interface has a routable IP address
			// This prevents false positives from interfaces that are up but not connected
			// (e.g., Tailscale interface that's up but not connected to the tailnet)
			if !hasRoutableIP(iface) {
				continue
			}

			// Get a friendly name from the interface if possible
			name := getVPNName(iface.Name, vpnType)
			return types.VPNInfo{
				IsConnected: true,
				Name:        name,
				Interface:   iface.Name,
				Type:        vpnType,
			}
		}
	}

	return types.VPNInfo{IsConnected: false}
}

// getVPNName tries to determine a friendly name for the VPN connection
func getVPNName(ifaceName, vpnType string) string {
	// For known VPN providers, return their name
	knownProviders := map[string]string{
		"tailscale": "Tailscale",
		"proton":    "ProtonVPN",
		"nordlynx":  "NordVPN",
		"mullvad":   "Mullvad",
	}

	for prefix, name := range knownProviders {
		if strings.HasPrefix(ifaceName, prefix) {
			return name
		}
	}

	// For WireGuard, the interface name is often the config name
	if vpnType == "wireguard" {
		return ifaceName
	}

	// Default to the VPN type
	return vpnType
}

func isConnected(iface string) bool {
	data, err := os.ReadFile(filepath.Join("/sys/class/net", iface, "operstate"))
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(data)) == "up"
}

func findPrimaryInterface() (net.Interface, string, error) {
	active, err := getAllActiveInterfaces()
	if err != nil {
		return net.Interface{}, "", err
	}

	// First pass: prioritize wired (non-WiFi, non-virtual) interfaces
	for _, iface := range active {
		if isWifiInterface(iface.Name) || isVirtualInterface(iface.Name) {
			continue
		}
		ip := getIPAddress(iface)
		if ip != "" && isConnected(iface.Name) {
			return iface, ip, nil
		}
	}

	// Second pass: fall back to WiFi interfaces (still excluding virtual)
	for _, iface := range active {
		if isVirtualInterface(iface.Name) {
			continue
		}
		ip := getIPAddress(iface)
		if ip != "" {
			return iface, ip, nil
		}
	}

	return net.Interface{}, "", errors.New("no connected interface found")
}

func getNetworkInfo() (*types.NetworkInfo, error) {
	iface, ip, err := findPrimaryInterface()
	if err != nil {
		return nil, err
	}

	counters, err := psnet.IOCounters(true)
	if err != nil {
		return nil, err
	}

	var c psnet.IOCountersStat
	for _, v := range counters {
		if v.Name == iface.Name {
			c = v
			break
		}
	}

	info := &types.NetworkInfo{
		Interface:   iface.Name,
		IPAddress:   ip,
		IsWifi:      isWifiInterface(iface.Name),
		IsConnected: isConnected(iface.Name),
		BytesRecv:   c.BytesRecv,
		BytesSent:   c.BytesSent,
		VPN:         getVPNInfo(),
	}

	if info.IsWifi {
		if ssid, strength, err := getWifiDetails(iface.Name); err == nil {
			info.SSID = ssid
			info.SignalStrength = strength
		}
	}
	return info, nil
}
