// Package gmcp implements the Generic MUD Communication Protocol
// (mudlet.org/manual). GMCP frames are out-of-band telnet
// subnegotiations (option 201) carrying JSON-encoded events that
// drive Mudlet's auto-mapper, vitals gauges, chat-capture panels,
// and the broader Mudlet package ecosystem.
//
// The package contains three concerns:
//   - Typed payload structs for outbound packages (this file).
//   - Manager: per-session subscription lifecycle + inbound
//     Core.* dispatch (manager.go + subscriptions.go).
//   - Builders that turn repo/event values into outbound payloads
//     (vitals.go, status.go, room_info.go, comm.go).
//
// Frame encoding lives in the telnet package (Session.WriteGMCP)
// because it's a single-method primitive that would otherwise pull
// telnet/iac.go into a cycle through this package.
package gmcp

// Standard outbound package names. Strings are the GMCP wire names;
// keep in sync with Mudlet's expected nomenclature.
const (
	PkgCharName        = "Char.Name"
	PkgCharVitals      = "Char.Vitals"
	PkgCharStatus      = "Char.Status"
	PkgRoomInfo        = "Room.Info"
	PkgCommChannelText = "Comm.Channel.Text"
	PkgCorePing        = "Core.Ping"
)

// Supported category prefixes. Mudlet's Core.Supports.Set entries
// like "Char 1" enable everything under the Char.* namespace;
// "Char.Vitals 1" enables just one package. Subscriber wire-up
// treats both forms as opt-in.
const (
	CatCore = "Core"
	CatChar = "Char"
	CatRoom = "Room"
	CatComm = "Comm"
)

// CharName is the one-shot package sent after login so Mudlet's UI
// header can show the character's display name.
type CharName struct {
	Name     string `json:"name"`
	Fullname string `json:"fullname,omitempty"`
}

// CharVitals carries the four numbers Mudlet's default gauges bind
// to. Stamina maps to "sp"/"maxsp" because mainstream client scripts
// expect those keys; the WheelMUD-internal name is Stamina (Phase L).
type CharVitals struct {
	HP    int32 `json:"hp"`
	MaxHP int32 `json:"maxhp"`
	SP    int32 `json:"sp"`
	MaxSP int32 `json:"maxsp"`
}

// CharStatus carries the rarely-changing identity fields that drive
// Mudlet's character header. Sent on login + level-up + death/respawn.
// Level is the d20 sum across ClassLevels; Class is the dominant
// class (highest level in the map, alphabetical tie-break).
type CharStatus struct {
	Name      string `json:"character_name"`
	Race      string `json:"race"`
	Class     string `json:"class"`
	Level     int    `json:"level"`
	Alignment string `json:"alignment,omitempty"`
}

// RoomInfo is the package Mudlet's auto-mapper consumes. Exits is a
// map of short-name (n/s/e/w/u/d/ne/nw/se/sw) to destination room
// number. Mudlet uses Num as the unique room id for its map graph.
type RoomInfo struct {
	Num   int64             `json:"num"`
	Name  string            `json:"name"`
	Zone  string            `json:"zone,omitempty"`
	Exits map[string]int64  `json:"exits"`
	Desc  string            `json:"desc,omitempty"`
	Extra map[string]string `json:"extra,omitempty"`
}

// CommChannelText is Mudlet's standard chat-capture package. Channel
// is the lowercase channel name ("say", "gossip", "ooc",
// "tell.in", "tell.out"). Talker is the speaker's display name.
type CommChannelText struct {
	Channel string `json:"channel"`
	Talker  string `json:"talker"`
	Text    string `json:"text"`
}
