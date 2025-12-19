package plugins

import (
	"fmt"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types/events"
)

// گروپ کمانڈز
func GroupCommands(cli *whatsmeow.Client, evt *events.Message) {
	// Kick, Add, Promote, Demote, TagAll
}

// ڈاؤن لوڈر کمانڈز
func DownloaderCommands(cli *whatsmeow.Client, evt *events.Message) {
	// IG, TikTok, FB, YouTube
}

// میڈیا ٹولز
func ToolCommands(cli *whatsmeow.Client, evt *events.Message) {
	// Ping, Sticker, Translate, Remini
}

// سیٹنگز
func SettingCommands(cli *whatsmeow.Client, evt *events.Message) {
	// AlwaysOnline, AutoStatus, ReadAllStatus
}