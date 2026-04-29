package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/GcZuRi1886/system-info-provider/types"
	"golang.org/x/sys/unix"
)

func findBacklightPath() (string, error) {
	const base = "/sys/class/backlight"
	entries, err := os.ReadDir(base)
	if err != nil {
		return "", fmt.Errorf("could not read %s: %w", base, err)
	}
	if len(entries) == 0 {
		return "", fmt.Errorf("no backlight devices found in %s", base)
	}
	return filepath.Join(base, entries[0].Name()), nil
}

func readBrightnessInt(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	var val int
	_, err = fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &val)
	return val, err
}

func listenForBrightnessChanges(emit func(dataType string, data any)) {
	backlightPath, err := findBacklightPath()
	if err != nil {
		log.Printf("Brightness: %v", err)
		return
	}

	brightnessFile := filepath.Join(backlightPath, "brightness")
	maxFile := filepath.Join(backlightPath, "max_brightness")

	maximum, err := readBrightnessInt(maxFile)
	if err != nil || maximum == 0 {
		log.Printf("Brightness: could not read max_brightness: %v", err)
		return
	}

	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC)
	if err != nil {
		log.Printf("Brightness: inotify_init failed: %v", err)
		return
	}
	defer unix.Close(fd)

	_, err = unix.InotifyAddWatch(fd, brightnessFile, unix.IN_MODIFY|unix.IN_CLOSE_WRITE)
	if err != nil {
		log.Printf("Brightness: inotify_add_watch failed: %v", err)
		return
	}

	log.Printf("Brightness: watching %s (max: %d)", brightnessFile, maximum)

	// Emit initial state
	if current, err := readBrightnessInt(brightnessFile); err == nil {
		emit("brightness", types.Wrapper{
			Type: "brightness",
			Data: &types.BrightnessInfo{
				Current: current,
				Maximum: maximum,
				Percentage:   float64(current) / float64(maximum),
			},
		})
	}

	buf := make([]byte, unix.SizeofInotifyEvent*16)
	for {
		n, err := unix.Read(fd, buf)
		if err != nil {
			log.Printf("Brightness: read error: %v", err)
			return
		}
		if n < unix.SizeofInotifyEvent {
			continue
		}

		current, err := readBrightnessInt(brightnessFile)
		if err != nil {
			log.Printf("Brightness: could not read brightness: %v", err)
			continue
		}

		emit("brightness", types.Wrapper{
			Type: "brightness",
			Data: &types.BrightnessInfo{
				Current: current,
				Maximum: maximum,
				Percentage:   float64(current) / float64(maximum),
			},
		})
	}
}
