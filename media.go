package main

import (
	"context"
	"encoding/json"
	"log"
	"net/url"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GcZuRi1886/system-info-provider/types"
	"github.com/godbus/dbus/v5"
)

const (
	mprisPrefix      = "org.mpris.MediaPlayer2."
	mprisPath        = dbus.ObjectPath("/org/mpris/MediaPlayer2")
	mprisPlayerIface = "org.mpris.MediaPlayer2.Player"
	mprisCallTimeout = 2 * time.Second
)

// isMprisPlayerName reports whether a bus name belongs to a real MPRIS player.
// playerctld is skipped since it only proxies another player.
func isMprisPlayerName(name string) bool {
	return strings.HasPrefix(name, mprisPrefix) && name != mprisPrefix+"playerctld"
}

// mprisPlayer holds the last known state of a single MPRIS player
type mprisPlayer struct {
	busName         string // well-known name, e.g. org.mpris.MediaPlayer2.spotify
	owner           string // unique name, e.g. :1.42 (signals are sent from this)
	status          string
	metadata        map[string]dbus.Variant
	position        int64 // microseconds
	positionUpdated time.Time
	canGoNext       bool
	canGoPrevious   bool
	canPlay         bool
	canPause        bool
	lastActive      time.Time
}

// Latest emitted media state, used for the initial snapshot on socket subscribe
var mediaCache struct {
	sync.RWMutex
	wrapper *types.Wrapper
}

func getCachedMediaState() types.Wrapper {
	mediaCache.RLock()
	defer mediaCache.RUnlock()
	if mediaCache.wrapper == nil {
		return types.Wrapper{Type: "media", Data: &types.MediaInfo{}}
	}
	return *mediaCache.wrapper
}

func listenForMediaChanges(emit func(dataType string, data any)) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		log.Printf("Media: failed to connect to session bus: %v", err)
		return
	}
	defer conn.Close()

	rules := []string{
		"type='signal',sender='org.freedesktop.DBus',interface='org.freedesktop.DBus',member='NameOwnerChanged',arg0namespace='org.mpris.MediaPlayer2'",
		"type='signal',interface='org.freedesktop.DBus.Properties',member='PropertiesChanged',path='/org/mpris/MediaPlayer2',arg0='org.mpris.MediaPlayer2.Player'",
		"type='signal',interface='org.mpris.MediaPlayer2.Player',member='Seeked',path='/org/mpris/MediaPlayer2'",
	}
	for _, rule := range rules {
		if call := conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, rule); call.Err != nil {
			log.Printf("Media: failed to add D-Bus match: %v", call.Err)
			return
		}
	}

	c := make(chan *dbus.Signal, 32)
	conn.Signal(c)

	players := make(map[string]*mprisPlayer) // bus name → player
	owners := make(map[string]string)        // unique name → bus name

	// Initial state
	var names []string
	if err := conn.BusObject().Call("org.freedesktop.DBus.ListNames", 0).Store(&names); err != nil {
		log.Printf("Media: failed to list bus names: %v", err)
	}
	for _, name := range names {
		if isMprisPlayerName(name) {
			addMprisPlayer(conn, players, owners, name, "")
		}
	}

	lastJSON := emitMediaState(emit, players, nil)

	for signalMsg := range c {
		handleMediaSignal(conn, signalMsg, players, owners)
		lastJSON = emitMediaState(emit, players, lastJSON)
	}
}

// addMprisPlayer registers a player and loads its current properties
func addMprisPlayer(conn *dbus.Conn, players map[string]*mprisPlayer, owners map[string]string, busName, owner string) {
	if owner == "" {
		if err := conn.BusObject().Call("org.freedesktop.DBus.GetNameOwner", 0, busName).Store(&owner); err != nil {
			log.Printf("Media: could not get owner of %s: %v", busName, err)
			return
		}
	}

	p := &mprisPlayer{busName: busName, owner: owner, metadata: map[string]dbus.Variant{}}

	ctx, cancel := context.WithTimeout(context.Background(), mprisCallTimeout)
	defer cancel()
	var props map[string]dbus.Variant
	err := conn.Object(busName, mprisPath).
		CallWithContext(ctx, "org.freedesktop.DBus.Properties.GetAll", 0, mprisPlayerIface).
		Store(&props)
	if err != nil {
		log.Printf("Media: could not read properties of %s: %v", busName, err)
	}

	applyPlayerProps(p, props)
	refreshPosition(conn, p)
	if p.status == "Playing" {
		p.lastActive = time.Now()
	}

	players[busName] = p
	owners[owner] = busName
	log.Printf("Media: tracking player %s", busName)
}

func removeMprisPlayer(players map[string]*mprisPlayer, owners map[string]string, busName string) {
	if p, ok := players[busName]; ok {
		delete(owners, p.owner)
		delete(players, busName)
		log.Printf("Media: player %s went away", busName)
	}
}

// Handle MPRIS related D-Bus signals
func handleMediaSignal(conn *dbus.Conn, signalMsg *dbus.Signal, players map[string]*mprisPlayer, owners map[string]string) {
	switch signalMsg.Name {
	case "org.freedesktop.DBus.NameOwnerChanged":
		if len(signalMsg.Body) < 3 {
			return
		}
		name, _ := signalMsg.Body[0].(string)
		oldOwner, _ := signalMsg.Body[1].(string)
		newOwner, _ := signalMsg.Body[2].(string)
		if !isMprisPlayerName(name) {
			return
		}
		if oldOwner != "" {
			removeMprisPlayer(players, owners, name)
		}
		if newOwner != "" {
			addMprisPlayer(conn, players, owners, name, newOwner)
		}

	case "org.freedesktop.DBus.Properties.PropertiesChanged":
		if len(signalMsg.Body) < 2 {
			return
		}
		if iface, _ := signalMsg.Body[0].(string); iface != mprisPlayerIface {
			return
		}
		p := playerForSender(players, owners, signalMsg.Sender)
		if p == nil {
			return
		}
		changedProps, _ := signalMsg.Body[1].(map[string]dbus.Variant)

		prevStatus := p.status
		prevTrack := trackKey(p.metadata)
		applyPlayerProps(p, changedProps)

		statusChanged := p.status != prevStatus
		trackChanged := trackKey(p.metadata) != prevTrack
		if statusChanged || trackChanged {
			// Position is not signalled via PropertiesChanged, so re-read it
			refreshPosition(conn, p)
			if p.status == "Playing" {
				p.lastActive = time.Now()
			}
		}

	case "org.mpris.MediaPlayer2.Player.Seeked":
		if len(signalMsg.Body) < 1 {
			return
		}
		p := playerForSender(players, owners, signalMsg.Sender)
		if p == nil {
			return
		}
		if pos, ok := signalMsg.Body[0].(int64); ok {
			p.position = pos
			p.positionUpdated = time.Now()
		}
	}
}

func playerForSender(players map[string]*mprisPlayer, owners map[string]string, sender string) *mprisPlayer {
	busName, ok := owners[sender]
	if !ok {
		return nil
	}
	return players[busName]
}

func applyPlayerProps(p *mprisPlayer, props map[string]dbus.Variant) {
	for k, v := range props {
		switch k {
		case "PlaybackStatus":
			p.status = getVariantString(v)
		case "Metadata":
			if md, ok := v.Value().(map[string]dbus.Variant); ok {
				p.metadata = md
			}
		case "Position":
			if pos, ok := variantInt64(v); ok {
				p.position = pos
				p.positionUpdated = time.Now()
			}
		case "CanGoNext":
			p.canGoNext = getVariantBool(v)
		case "CanGoPrevious":
			p.canGoPrevious = getVariantBool(v)
		case "CanPlay":
			p.canPlay = getVariantBool(v)
		case "CanPause":
			p.canPause = getVariantBool(v)
		}
	}
}

func refreshPosition(conn *dbus.Conn, p *mprisPlayer) {
	ctx, cancel := context.WithTimeout(context.Background(), mprisCallTimeout)
	defer cancel()
	var v dbus.Variant
	err := conn.Object(p.busName, mprisPath).
		CallWithContext(ctx, "org.freedesktop.DBus.Properties.Get", 0, mprisPlayerIface, "Position").
		Store(&v)
	if err != nil {
		// Not every player supports Position
		return
	}
	if pos, ok := variantInt64(v); ok {
		p.position = pos
		p.positionUpdated = time.Now()
	}
}

// trackKey identifies the current track to detect track changes
func trackKey(md map[string]dbus.Variant) string {
	if id, ok := md["mpris:trackid"]; ok {
		if path, ok := id.Value().(dbus.ObjectPath); ok {
			return string(path)
		}
		if s, ok := id.Value().(string); ok {
			return s
		}
	}
	return metadataString(md, "xesam:title") + "\x00" + metadataArtist(md)
}

// selectActivePlayer prefers a playing player, then the most recently active one
func selectActivePlayer(players map[string]*mprisPlayer) *mprisPlayer {
	list := make([]*mprisPlayer, 0, len(players))
	for _, p := range players {
		list = append(list, p)
	}
	if len(list) == 0 {
		return nil
	}
	sort.Slice(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if (a.status == "Playing") != (b.status == "Playing") {
			return a.status == "Playing"
		}
		if !a.lastActive.Equal(b.lastActive) {
			return a.lastActive.After(b.lastActive)
		}
		return a.busName < b.busName
	})
	return list[0]
}

func buildMediaInfo(p *mprisPlayer) *types.MediaInfo {
	if p == nil {
		return &types.MediaInfo{}
	}
	info := &types.MediaInfo{
		Active:        true,
		Player:        strings.TrimPrefix(p.busName, mprisPrefix),
		Status:        p.status,
		Title:         metadataString(p.metadata, "xesam:title"),
		Artist:        metadataArtist(p.metadata),
		Album:         metadataString(p.metadata, "xesam:album"),
		ArtURL:        metadataString(p.metadata, "mpris:artUrl"),
		Position:      float64(p.position) / 1e6,
		CanGoNext:     p.canGoNext,
		CanGoPrevious: p.canGoPrevious,
		CanPlay:       p.canPlay,
		CanPause:      p.canPause,
	}
	if info.Title == "" {
		// Untagged local files often only provide a URL
		if u, err := url.Parse(metadataString(p.metadata, "xesam:url")); err == nil && u.Path != "" {
			info.Title = path.Base(u.Path)
		}
	}
	if length, ok := p.metadata["mpris:length"]; ok {
		if l, ok := variantInt64(length); ok {
			info.Length = float64(l) / 1e6
		}
	}
	if !p.positionUpdated.IsZero() {
		info.PositionUpdated = p.positionUpdated.UnixMilli()
	}
	return info
}

// emitMediaState emits the active player's state if it differs from the last emitted one
func emitMediaState(emit func(dataType string, data any), players map[string]*mprisPlayer, lastJSON []byte) []byte {
	wrapper := types.Wrapper{
		Type: "media",
		Data: buildMediaInfo(selectActivePlayer(players)),
	}
	data, err := json.Marshal(wrapper)
	if err != nil {
		log.Printf("Media: failed to marshal state: %v", err)
		return lastJSON
	}
	if lastJSON != nil && string(data) == string(lastJSON) {
		return lastJSON
	}

	mediaCache.Lock()
	mediaCache.wrapper = &wrapper
	mediaCache.Unlock()

	emit(wrapper.Type, wrapper)
	return data
}

func metadataString(md map[string]dbus.Variant, key string) string {
	if v, ok := md[key]; ok {
		return getVariantString(v)
	}
	return ""
}

func metadataArtist(md map[string]dbus.Variant) string {
	v, ok := md["xesam:artist"]
	if !ok {
		return ""
	}
	switch val := v.Value().(type) {
	case []string:
		return strings.Join(val, ", ")
	case string:
		return val
	}
	return ""
}

// variantInt64 handles the various integer types players use for times
func variantInt64(v dbus.Variant) (int64, bool) {
	switch val := v.Value().(type) {
	case int64:
		return val, true
	case uint64:
		return int64(val), true
	case int32:
		return int64(val), true
	case uint32:
		return int64(val), true
	case float64:
		return int64(val), true
	}
	return 0, false
}
