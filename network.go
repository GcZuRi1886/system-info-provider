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

	// First pass: prioritize wired (non-WiFi) interfaces
	for _, iface := range active {
		if isWifiInterface(iface.Name) {
			continue
		}
		ip := getIPAddress(iface)
		if ip != "" && isConnected(iface.Name) {
			return iface, ip, nil
		}
	}

	// Second pass: fall back to WiFi interfaces
	for _, iface := range active {
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
	}

	if info.IsWifi {
		if ssid, strength, err := getWifiDetails(iface.Name); err == nil {
			info.SSID = ssid
			info.SignalStrength = strength
		}
	}
	return info, nil
}
