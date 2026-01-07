package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E" // 🟢 NEW PATH (Research Verified)
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

// 🎛️ MAIN SWITCH HANDLER
func HandleButtonCommands(client *whatsmeow.Client, evt *events.Message) {
	text := evt.Message.GetConversation()
	if text == "" {
		text = evt.Message.GetExtendedTextMessage().GetText()
	}

	if !strings.HasPrefix(strings.ToLower(text), ".btn") {
		return
	}

	chatJID := evt.Info.Chat
	cmd := strings.TrimSpace(strings.ToLower(text))

	switch cmd {
	case ".btn 1":
		// 🔥 COPY CODE BUTTON
		fmt.Println("Testing Copy Button...")
		params := map[string]string{
			"display_text": "👉 Copy Code",
			"copy_code":    "IMPOSSIBLE-2026",
		}
		sendNativeFlow(client, chatJID, "🔥 *Copy Button Test*", "نیچے بٹن دبا کر کوڈ کاپی کریں۔", "cta_copy", params)

	case ".btn 2":
		// 🌍 URL BUTTON
		fmt.Println("Testing URL Button...")
		params := map[string]string{
			"display_text": "🌐 Open Google",
			"url":          "https://google.com",
			"merchant_url": "https://google.com",
		}
		sendNativeFlow(client, chatJID, "🌍 *URL Button Test*", "ہماری ویب سائٹ وزٹ کریں۔", "cta_url", params)

	case ".btn 3":
		// 📜 LIST MENU (Single Select)
		fmt.Println("Testing List Menu...")
		
		// List JSON Structure
		listParams := map[string]interface{}{
			"title": "✨ Select Option",
			"sections": []map[string]interface{}{
				{
					"title": "Main Features",
					"rows": []map[string]string{
						{"header": "🤖", "title": "AI Chat", "description": "Chat with Gemini", "id": "row_ai"},
						{"header": "📥", "title": "Downloader", "description": "Download Videos", "id": "row_dl"},
					},
				},
				{
					"title": "Settings",
					"rows": []map[string]string{
						{"header": "⚙️", "title": "Panel", "description": "Admin Controls", "id": "row_panel"},
					},
				},
			},
		}
		sendNativeFlow(client, chatJID, "📂 *List Menu Test*", "نیچے مینیو کھولیں۔", "single_select", listParams)

	default:
		menu := "🛠️ *BUTTON TESTER MENU (New Lib)*\n\n" +
			"➤ `.btn 1` : Copy Code Button\n" +
			"➤ `.btn 2` : Open URL Button\n" +
			"➤ `.btn 3` : List Menu\n"
		
		client.SendMessage(context.Background(), chatJID, &waE2E.Message{
			Conversation: proto.String(menu),
		})
	}
}

// ---------------------------------------------------------
// 👇 HELPER FUNCTIONS (UPDATED FOR waE2E)
// ---------------------------------------------------------

func sendNativeFlow(client *whatsmeow.Client, jid types.JID, title string, body string, btnName string, params interface{}) {
	// JSON Marshal (Safe way)
	jsonBytes, err := json.Marshal(params)
	if err != nil {
		fmt.Println("JSON Error:", err)
		return
	}

	// 1. بٹن تیار کریں
	buttons := []*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{
		{
			Name:             proto.String(btnName),
			ButtonParamsJson: proto.String(string(jsonBytes)), // Note: waE2E uses Json (not JSON)
		},
	}

	// 2. میسج اسٹرکچر (Using waE2E & FutureProofMessage as per research)
	msg := &waE2E.Message{
		ViewOnceMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				InteractiveMessage: &waE2E.InteractiveMessage{
					Header: &waE2E.InteractiveMessage_Header{
						Title:              proto.String(title),
						HasMediaAttachment: proto.Bool(false),
					},
					Body: &waE2E.InteractiveMessage_Body{
						Text: proto.String(body),
					},
					Footer: &waE2E.InteractiveMessage_Footer{
						Text: proto.String("🤖 Impossible Bot Beta"),
					},
					
					// ✅ Native Flow Wrapper for waE2E
					InteractiveMessage: &waE2E.InteractiveMessage_NativeFlowMessage_{
						NativeFlowMessage: &waE2E.InteractiveMessage_NativeFlowMessage{
							Buttons:        buttons,
							MessageVersion: proto.Int32(3), // Version 3 is critical
						},
					},
				},
			},
		},
	}

	// 3. سینڈ کریں
	_, err = client.SendMessage(context.Background(), jid, msg)
	if err != nil {
		fmt.Println("❌ Error sending buttons:", err)
	}
}
