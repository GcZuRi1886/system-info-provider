package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
)

// Rotate the log file on startup once it grows past this size
const maxLogSize = 5 * 1024 * 1024

func getLogPath() string {
	stateDir := os.Getenv("XDG_STATE_HOME")
	if stateDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = os.TempDir()
		}
		stateDir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(stateDir, "system-info-provider", "system-info-provider.log")
}

var logPath = getLogPath()

// setupLogging sends log output to both stderr and the log file.
// Every running instance appends to the same file, so lines are prefixed with the mode.
func setupLogging(mode string) {
	log.SetPrefix(fmt.Sprintf("[%s] ", mode))
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)

	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		log.Printf("Could not create log directory: %v", err)
		return
	}
	if info, err := os.Stat(logPath); err == nil && info.Size() > maxLogSize {
		os.Rename(logPath, logPath+".1")
	}

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		log.Printf("Could not open log file %s: %v", logPath, err)
		return
	}
	log.SetOutput(io.MultiWriter(os.Stderr, f))
}

// printLogs writes the log file to stdout. With follow it keeps
// streaming new lines, like `tail -f`, and survives rotation.
func printLogs(follow bool) error {
	f, err := os.Open(logPath)
	if err != nil && !(follow && os.IsNotExist(err)) {
		return err
	}
	if f != nil {
		if _, err := io.Copy(os.Stdout, f); err != nil {
			return err
		}
	}
	if !follow {
		return f.Close()
	}

	for {
		time.Sleep(250 * time.Millisecond)

		info, err := os.Stat(logPath)
		if err != nil {
			continue // file gone mid-rotation; wait for it to reappear
		}

		if f != nil {
			cur, statErr := f.Stat()
			pos, _ := f.Seek(0, io.SeekCurrent)
			replaced := statErr == nil && !os.SameFile(cur, info)
			if !replaced {
				if info.Size() < pos {
					f.Seek(0, io.SeekStart) // truncated
				}
				io.Copy(os.Stdout, f)
				continue
			}
			// Rotated: drain what was left in the old file before switching
			io.Copy(os.Stdout, f)
			f.Close()
		}

		if f, err = os.Open(logPath); err != nil {
			f = nil
			continue
		}
		io.Copy(os.Stdout, f)
	}
}
