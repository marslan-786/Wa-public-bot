package main

import (
	"context"
	"fmt"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waProto "go.mau.fi/whatsmeow/binary/proto"
)

// ==================== سیکورٹی سسٹم ====================
func checkSecurity(client *whatsmeow.Client, v *events.Message) {
	if !v.Info.IsGroup {
		return
	}

	s := getGroupSettings(v.Info.Chat.String())
	if s.Mode == "private" {
		return
	}

	// Anti-link check
	if s.Antilink && containsLink(getText(v.Message)) {
		if s.AntilinkAdmin && isAdmin(client, v.Info.Chat, v.Info.Sender) {
			return
		}
		takeSecurityAction(client, v, s.AntilinkAction, "Link detected")
		return
	}

	// Anti-picture check
	if s.AntiPic && v.Message.ImageMessage != nil {
		if s.AntilinkAdmin && isAdmin(client, v.Info.Chat, v.Info.Sender) {
			return
		}
		takeSecurityAction(client, v, "delete", "Image not allowed")
		return
	}

	// Anti-video check
	if s.AntiVideo && v.Message.VideoMessage != nil {
		if s.AntilinkAdmin && isAdmin(client, v.Info.Chat, v.Info.Sender) {
			return
		}
		takeSecurityAction(client, v, "delete", "Video not allowed")
		return
	}

	// Anti-sticker check
	if s.AntiSticker && v.Message.StickerMessage != nil {
		if s.AntilinkAdmin && isAdmin(client, v.Info.Chat, v.Info.Sender) {
			return
		}
		takeSecurityAction(client, v, "delete", "Sticker not allowed")
		return
	}
}

func containsLink(text string) bool {
	if text == "" {
		return false
	}

	text = strings.ToLower(text)
	linkPatterns := []string{
		"http://", "https://", "www.",
		"chat.whatsapp.com/", "t.me/", "youtube.com/",
		"youtu.be/", "instagram.com/", "fb.com/",
		"facebook.com/", "twitter.com/", "x.com/",
	}

	for _, pattern := range linkPatterns {
		if strings.Contains(text, pattern) {
			return true
		}
	}

	return false
}

func takeSecurityAction(client *whatsmeow.Client, v *events.Message, action, reason string) {
	switch action {
	case "delete":
		// ✅ FIXED: Use RevokeMessage instead of DeleteMessage
		client.RevokeMessage(context.Background(), v.Info.Chat, v.Info.ID)
		msg := fmt.Sprintf(`╔═══════════════════════════╗
║   🚫 MESSAGE DELETED        ║
╠═══════════════════════════╣
║                           ║
║  ⚠️ *Reason:*              ║
║     %s                    ║
║                           ║
║  👤 *User:* @%s           ║
║                           ║
╚═══════════════════════════╝`, reason, v.Info.Sender.User)
		
		client.SendMessage(context.Background(), v.Info.Chat, &waProto.Message{
			ExtendedTextMessage: &waProto.ExtendedTextMessage{
				Text: &msg,
				ContextInfo: &waProto.ContextInfo{
					MentionedJID: []string{v.Info.Sender.String()},
				},
			},
		})

	case "deletekick":
		// ✅ FIXED: Use RevokeMessage instead of DeleteMessage
		client.RevokeMessage(context.Background(), v.Info.Chat, v.Info.ID)
		client.UpdateGroupParticipants(context.Background(), v.Info.Chat,
			[]types.JID{v.Info.Sender}, whatsmeow.ParticipantChangeRemove)
		
		msg := fmt.Sprintf(`╔═══════════════════════════╗
║   👢 USER KICKED            ║
╠═══════════════════════════╣
║                           ║
║  ⚠️ *Reason:*              ║
║     %s                    ║
║                           ║
║  👤 *User:* @%s           ║
║  🗑️ *Action:* Delete + Kick║
║                           ║
╚═══════════════════════════╝`, reason, v.Info.Sender.User)
		
		client.SendMessage(context.Background(), v.Info.Chat, &waProto.Message{
			ExtendedTextMessage: &waProto.ExtendedTextMessage{
				Text: &msg,
				ContextInfo: &waProto.ContextInfo{
					MentionedJID: []string{v.Info.Sender.String()},
				},
			},
		})

	case "deletewarn":
		s := getGroupSettings(v.Info.Chat.String())
		senderKey := v.Info.Sender.String()

		s.Warnings[senderKey]++
		warnCount := s.Warnings[senderKey]

		// ✅ FIXED: Use RevokeMessage instead of DeleteMessage
		client.RevokeMessage(context.Background(), v.Info.Chat, v.Info.ID)

		if warnCount >= 3 {
			client.UpdateGroupParticipants(context.Background(), v.Info.Chat,
				[]types.JID{v.Info.Sender}, whatsmeow.ParticipantChangeRemove)
			delete(s.Warnings, senderKey)
			
			msg := fmt.Sprintf(`╔═══════════════════════════╗
║   🚫 USER KICKED (3 WARNS)  ║
╠═══════════════════════════╣
║                           ║
║  👤 *User:* @%s           ║
║  ⚠️ *Final Warning:* 3/3  ║
║                           ║
║  🔨 *Action:* Kicked Out  ║
║                           ║
╚═══════════════════════════╝`, v.Info.Sender.User)
			
			client.SendMessage(context.Background(), v.Info.Chat, &waProto.Message{
				ExtendedTextMessage: &waProto.ExtendedTextMessage{
					Text: &msg,
					ContextInfo: &waProto.ContextInfo{
						MentionedJID: []string{v.Info.Sender.String()},
					},
				},
			})
		} else {
			msg := fmt.Sprintf(`╔═══════════════════════════╗
║   ⚠️ WARNING ISSUED         ║
╠═══════════════════════════╣
║                           ║
║  👤 *User:* @%s           ║
║  📊 *Warning:* %d/3       ║
║                           ║
║  🚨 *Reason:*             ║
║     %s                    ║
║                           ║
║  ⚠️ 3 warnings = Kick     ║
║                           ║
╚═══════════════════════════╝`, v.Info.Sender.User, warnCount, reason)
			
			client.SendMessage(context.Background(), v.Info.Chat, &waProto.Message{
				ExtendedTextMessage: &waProto.ExtendedTextMessage{
					Text: &msg,
					ContextInfo: &waProto.ContextInfo{
						MentionedJID: []string{v.Info.Sender.String()},
					},
				},
			})
		}

		saveGroupSettings(s)
	}
}

func startSecuritySetup(client *whatsmeow.Client, v *events.Message, secType string) {
	if !v.Info.IsGroup {
		msg := `╔═══════════════════════════╗
║    ❌ GROUP ONLY COMMAND   ║
╠═══════════════════════════╣
║  This command works only  ║
║  in group chats           ║
╚═══════════════════════════╝`
		replyMessage(client, v, msg)
		return
	}

	if !isAdmin(client, v.Info.Chat, v.Info.Sender) && !isOwner(client, v.Info.Sender) {
		msg := `╔═══════════════════════════╗
║      ❌ ACCESS DENIED      ║
╠═══════════════════════════╣
║  🔒 Admin Only Command    ║
╚═══════════════════════════╝`
		replyMessage(client, v, msg)
		return
	}

	setupMap[v.Info.Sender.String()] = &SetupState{
		Type:    secType,
		Stage:   1,
		GroupID: v.Info.Chat.String(),
		User:    v.Info.Sender.String(),
	}

	msg := fmt.Sprintf(`╔═══════════════════════════╗
║  🛡️ %s SETUP (1/2)         ║
╠═══════════════════════════╣
║                           ║
║  ❓ *Allow Admins?*       ║
║                           ║
║  Should admins be allowed ║
║  to bypass this security? ║
║                           ║
║  1️⃣ Reply: *1* for YES    ║
║  2️⃣ Reply: *2* for NO     ║
║                           ║
╚═══════════════════════════╝`, strings.ToUpper(secType))

	replyMessage(client, v, msg)
}

func handleSetupResponse(client *whatsmeow.Client, v *events.Message, state *SetupState) {
	txt := strings.TrimSpace(getText(v.Message))
	s := getGroupSettings(state.GroupID)

	if state.Stage == 1 {
		if txt == "1" {
			s.AntilinkAdmin = true
		} else if txt == "2" {
			s.AntilinkAdmin = false
		} else {
			msg := `╔═══════════════════════════╗
║    ❌ INVALID RESPONSE     ║
╠═══════════════════════════╣
║  Please reply with:       ║
║  1️⃣ for YES               ║
║  2️⃣ for NO                ║
╚═══════════════════════════╝`
			replyMessage(client, v, msg)
			return
		}
		state.Stage = 2

		msg := fmt.Sprintf(`╔═══════════════════════════╗
║  ⚡ %s SETUP (2/2)         ║
╠═══════════════════════════╣
║                           ║
║  🎯 *Choose Action:*      ║
║                           ║
║  1️⃣ *DELETE ONLY*         ║
║     Just remove message   ║
║                           ║
║  2️⃣ *DELETE + KICK*       ║
║     Remove & kick user    ║
║                           ║
║  3️⃣ *DELETE + WARN*       ║
║     Warn (kick at 3)      ║
║                           ║
║  Reply with 1, 2, or 3    ║
║                           ║
╚═══════════════════════════╝`, strings.ToUpper(state.Type))

		replyMessage(client, v, msg)
		return
	}

	if state.Stage == 2 {
		var actionText string
		switch txt {
		case "1":
			s.AntilinkAction = "delete"
			actionText = "Delete Only"
		case "2":
			s.AntilinkAction = "deletekick"
			actionText = "Delete + Kick"
		case "3":
			s.AntilinkAction = "deletewarn"
			actionText = "Delete + Warn"
		default:
			msg := `╔═══════════════════════════╗
║    ❌ INVALID RESPONSE     ║
╠═══════════════════════════╣
║  Please reply with:       ║
║  1️⃣ for Delete Only       ║
║  2️⃣ for Delete + Kick     ║
║  3️⃣ for Delete + Warn     ║
╚═══════════════════════════╝`
			replyMessage(client, v, msg)
			return
		}

		switch state.Type {
		case "antilink":
			s.Antilink = true
		case "antipic":
			s.AntiPic = true
		case "antivideo":
			s.AntiVideo = true
		case "antisticker":
			s.AntiSticker = true
		}

		saveGroupSettings(s)
		delete(setupMap, state.User)

		adminAllow := "YES ✅"
		if !s.AntilinkAdmin {
			adminAllow = "NO ❌"
		}

		msg := fmt.Sprintf(`╔═══════════════════════════╗
║  ✅ %s ENABLED              ║
╠═══════════════════════════╣
║                           ║
║  🛡️ *Feature:* %s         ║
║  👑 *Admin Allow:* %s     ║
║  ⚡ *Action:* %s           ║
║                           ║
║  ✅ *Successfully Configured*║
║                           ║
╚═══════════════════════════╝`,
			strings.ToUpper(state.Type),
			strings.ToUpper(state.Type),
			adminAllow,
			actionText)

		replyMessage(client, v, msg)
	}
}