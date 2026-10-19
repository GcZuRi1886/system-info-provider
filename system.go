package main

import (
	"bufio"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/GcZuRi1886/system-info-provider/types"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
)

var systemInfo types.CurrentStateData
var systemInfoWrapper types.Wrapper


// ----- periodic system info -----
func sysInfoLoop(emit func(dataType string, data any)) {
	systemInfoWrapper.Type = "system"
	systemInfoWrapper.Data = &systemInfo
	for {
		// Time
		now := time.Now().Format("Mon 01 Jan 15:04:05")

		// CPU usage 
		cpuPercent, _ := cpu.Percent(0, true)
		cpuUsage := cpuPercent // per core
		avgPercent, _ := cpu.Percent(0, false)

		// Memory usage
		vm, _ := mem.VirtualMemory()
		totalMem := vm.Total
		usedMem := vm.Used

		// Update battery info
		batteryInfo := getBatteryInfo()
		
		// Audio info
		//audioInfo, err := GetAudioInfo()
		//if err != nil {
		//	fmt.Println("Error getting audio info:", err)
		//}

		networkinfo, err := getNetworkInfo()
		if err != nil {
			log.Println("Error getting network info:", err)
		}

		systemInfo.Time = now
		systemInfo.CPUPerCore = cpuUsage
		systemInfo.CPUAverage = avgPercent[0]
		systemInfo.MemoryUsed = int(usedMem)
		systemInfo.MemoryTotal = int(totalMem)
		systemInfo.Battery = *batteryInfo
		systemInfo.Network = *networkinfo
		emit(systemInfoWrapper.Type, systemInfoWrapper)

		time.Sleep(3 * time.Second)
	}
}

// ----- get battery info -----
func getBatteryInfo() *types.BatteryInfo {
	batteryBase := "/sys/class/power_supply/BAT0/"
	
	batteryInfo, err := os.Open(batteryBase + "uevent")
	if err != nil {
		return &types.BatteryInfo{}
	}
	defer batteryInfo.Close()

	batteryData := make(map[string]string)
	scanner := bufio.NewScanner(batteryInfo)
	
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			batteryData[parts[0]] = parts[1]
		}
	}
	

	if len(batteryData) > 0 {
		chargeRate, _ := strconv.ParseFloat(batteryData["POWER_SUPPLY_POWER_NOW"], 64)
		currentCapacity, _ := strconv.ParseFloat(batteryData["POWER_SUPPLY_ENERGY_NOW"], 64)
		fullCapacity, _ := strconv.ParseFloat(batteryData["POWER_SUPPLY_ENERGY_FULL"], 64)
		currentPercentage, _ := strconv.Atoi(batteryData["POWER_SUPPLY_CAPACITY"])
		status := batteryData["POWER_SUPPLY_STATUS"]
		if chargeRate == 0 {
			chargeRate = 1 // to avoid division by zero
		}
		battery := &types.BatteryInfo{
			Percentage:  currentPercentage,
			State:       status,
			TimeToEmpty: currentCapacity / chargeRate * 60, // in minutes
			TimeToFull:  (fullCapacity - currentCapacity) / chargeRate * 60, // in minutes
		}
		return battery
	}
	return &types.BatteryInfo{}
}
