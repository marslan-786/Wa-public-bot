package plugins

import (
	"context"
	"fmt"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func init() {
	Register("kick", "Group", func(cli *whatsmeow.Client, evt *events.Message, args []string) {
		target, _ := types.ParseJID(args[0] + "@s.whatsapp.net")
		cli.UpdateGroupParticipants(evt.Info.Chat, []types.JID{target}, whatsmeow.GroupParticipantChangeRemove)
		fmt.Println("User Kicked")
	})

	Register("tagall", "Group", func(cli *whatsmeow.Client, evt *events.Message, args []string) {
		meta, _ := cli.GetGroupInfo(evt.Info.Chat)
		// ٹیگ آل لاجک
	})
    // ... دیگر کمانڈز (Add, Promote, Demote, Group Close)
}