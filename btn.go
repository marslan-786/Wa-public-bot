package main

import (
	"context"
	"fmt"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
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

	cmd := strings.TrimSpace(strings.ToLower(text))

	// 🛠️ ہیڈر اور باڈی ٹیکسٹ (جو ہر میسج میں جائے گا)
	headerText := "🤖 Impossible Bot"
	bodyText := "براہ کرم نیچے دیئے گئے بٹن پر کلک کریں۔"
	footerText := "Powered by Whatsmeow"

	switch cmd {
	case ".btn 1":
		// 🔥 COPY CODE BUTTON (cta_copy)
		// JSON Payload must have 'display_text' and 'copy_code'
		fmt.Println("🚀 Sending Copy Button...")
		jsonPayload := `{"display_text":"👉 Copy Code","copy_code":"IMPOSSIBLE-2026","id":"btn_copy_123"}`
		
		sendNativeFlow(client, evt, headerText, bodyText, footerText, "cta_copy", jsonPayload)

	case ".btn 2":
		// 🌍 URL BUTTON (cta_url)
		// Must include 'url' and 'merchant_url' for compatibility
		fmt.Println("🚀 Sending URL Button...")
		jsonPayload := `{"display_text":"🌐 Open Google","url":"https://google.com","merchant_url":"https://google.com","id":"btn_url_456"}`
		
		sendNativeFlow(client, evt, headerText, bodyText, footerText, "cta_url", jsonPayload)

	case ".btn 3":
		// 📜 LIST MENU (single_select)
		// Strictly formatted JSON structure for List Messages
		fmt.Println("🚀 Sending List Menu...")
		jsonPayload := `{
			"title": "✨ Select Option",
			"sections": [
				{
					"title": "Main Features",
					"rows": [
						{"header": "🤖", "title": "AI Chat", "description": "Chat with Gemini", "id": "row_ai"},
						{"header": "📥", "title": "Downloader", "description": "Save Videos", "id": "row_dl"}
					]
				},
				{
					"title": "Settings",
					"rows": [
						{"header": "⚙️", "title": "Admin Panel", "description": "Manage Bot", "id": "row_panel"}
					]
				}
			]
		}`
		
		sendNativeFlow(client, evt, headerText, bodyText, footerText, "single_select", jsonPayload)
	}
}

// ---------------------------------------------------------
// 👇 HELPER FUNCTION (DEEP SEARCH COMPLIANT WRAPPER)
// ---------------------------------------------------------

func sendNativeFlow(client *whatsmeow.Client, evt *events.Message, header, body, footer, btnName, jsonParams string) {
	
	// 1. Button Structure
	buttons := []*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{
		{
			Name:             proto.String(btnName),
			ButtonParamsJSON: proto.String(jsonParams),
		},
	}

	// 2. Message Structure (The Deep Search Approved Format)
	// ViewOnceMessage -> FutureProofMessage -> InteractiveMessage
	msg := &waE2E.Message{
		ViewOnceMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				InteractiveMessage: &waE2E.InteractiveMessage{
					Header: &waE2E.InteractiveMessage_Header{
						Title:              proto.String(header),
						Subtitle:           proto.String("Authorized Action"), // Some clients need this
						HasMediaAttachment: proto.Bool(false),
					},
					Body: &waE2E.InteractiveMessage_Body{
						Text: proto.String(body),
					},
					Footer: &waE2E.InteractiveMessage_Footer{
						Text: proto.String(footer),
					},
					
					// ✅ Native Flow Wrapper
					InteractiveMessage: &waE2E.InteractiveMessage_NativeFlowMessage_{
						NativeFlowMessage: &waE2E.InteractiveMessage_NativeFlowMessage{
							Buttons:           buttons,
							// 🛑 CRITICAL: This MUST be a valid JSON string (even if empty object)
							MessageParamsJSON: proto.String("{\"name\":\"galaxy_message\"}"), 
							MessageVersion:    proto.Int32(3), // Version 3 is standard for Native Flow
						},
					},

					// 🔥 Reply Context (Essential for Visibility)
					ContextInfo: &waE2E.ContextInfo{
						StanzaID:      proto.String(evt.Info.ID),
						Participant:   proto.String(evt.Info.Sender.String()),
						QuotedMessage: evt.Message,
					},
				},
			},
		},
	}

	// 3. Send & Log
	fmt.Printf("📦 Sending Native Flow (%s)...\n", btnName)
	resp, err := client.SendMessage(context.Background(), evt.Info.Chat, msg)
	
	if err != nil {
		fmt.Printf("❌ Error sending: %v\n", err)
	} else {
		fmt.Printf("✅ Sent! ID: %s | TS: %v\n", resp.ID, resp.Timestamp)
	}
}
