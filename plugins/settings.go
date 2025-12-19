package plugins

import (
	"fmt"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types/events"
)

func init() {
	// Always Online
	Register("alwaysonline", "Settings", func(cli *whatsmeow.Client, evt *events.Message, args []string) {
		if len(args) == 0 { return }
		cli.SendPresenceUpdate(whatsmeow.PresenceAvailable)
		fmt.Println("Presence updated to Online")
	})

	// Auto Status View
	Register("autostatus", "Settings", func(cli *whatsmeow.Client, evt *events.Message, args []string) {
		// آٹو اسٹیٹس ریڈ لاجک
	})

	// Antilink, Antipic, Antivideo
}