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

	// 🛠️ چینل کا نام (جو میسج کے اوپر نظر آئے گا)
	channelName := "Impossible Updates 🚀"
	
	// 🛠️ ہیڈر اور باڈی ٹیکسٹ
	headerText := "🤖 Impossible Bot"
	footerText := "Powered by Whatsmeow"

	// 3. اب صرف نمبر چیک کریں (Command Logic)
	// پہلے ہم ".btn 1" چیک کر رہے تھے، لیکن یہاں ہم پورا ٹیکسٹ ہی میچ کر لیتے ہیں
	// یا پھر اگر آپ نے پچھلی لاجک (strings.Fields) استعمال کرنی ہے تو وہ بھی ٹھیک ہے
	// فی الحال میں سادگی کے لیے direct match کر رہا ہوں
	
	if cmd == ".btn 1" {
		fmt.Println("🚀 Sending Copy Button...")
		jsonPayload := `{"display_text":"👉 Copy Code","copy_code":"IMPOSSIBLE-2026","id":"btn_copy_123"}`
		sendNativeFlow(client, evt, headerText, "نیچے بٹن دبا کر کوڈ کاپی کریں۔", footerText, "cta_copy", jsonPayload, channelName)
	} else if cmd == ".btn 2" {
		fmt.Println("🚀 Sending URL Button...")
		jsonPayload := `{"display_text":"🌐 Open Google","url":"https://google.com","merchant_url":"https://google.com","id":"btn_url_456"}`
		sendNativeFlow(client, evt, headerText, "ہماری ویب سائٹ وزٹ کریں۔", footerText, "cta_url", jsonPayload, channelName)
	} else if cmd == ".btn 3" {
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
				}
			]
		}`
		sendNativeFlow(client, evt, headerText, "نیچے مینیو کھولیں۔", footerText, "single_select", jsonPayload, channelName)
	} else {
		// 🛠️ اگر صرف .btn لکھا ہو تو یہ چلے گا
		fmt.Println("🚀 Sending Default Help (Simple Forward)...")
		
		helpBody := "🛠️ *BUTTON TESTER MENU*\n\n" +
			"➤ `.btn 1` : Copy Code Button\n" +
			"➤ `.btn 2` : Open URL Button\n" +
			"➤ `.btn 3` : List Menu\n\n" +
			"⚠️ *Note:* Forwarded Tag Check."
		
		sendSimpleChannelForward(client, evt, helpBody, channelName)
	}
}

// ---------------------------------------------------------
// 👇 HELPER FUNCTION 1: NATIVE FLOW WITH CHANNEL FORWARD
// ---------------------------------------------------------

func sendNativeFlow(client *whatsmeow.Client, evt *events.Message, title, body, footer, btnName, jsonParams, channelName string) {
	
	buttons := []*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{
		{
			Name:             proto.String(btnName),
			ButtonParamsJSON: proto.String(jsonParams),
		},
	}

	msg := &waE2E.Message{
		ViewOnceMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				InteractiveMessage: &waE2E.InteractiveMessage{
					Header: &waE2E.InteractiveMessage_Header{
						Title:              proto.String(title),
						Subtitle:           proto.String(channelName),
						HasMediaAttachment: proto.Bool(false),
					},
					Body: &waE2E.InteractiveMessage_Body{
						Text: proto.String(body),
					},
					Footer: &waE2E.InteractiveMessage_Footer{
						Text: proto.String(footer),
					},
					InteractiveMessage: &waE2E.InteractiveMessage_NativeFlowMessage_{
						NativeFlowMessage: &waE2E.InteractiveMessage_NativeFlowMessage{
							Buttons:           buttons,
							// 🔥 CRITICAL: Galaxy Message Marker
							MessageParamsJSON: proto.String("{\"name\":\"galaxy_message\"}"), 
							MessageVersion:    proto.Int32(3),
						},
					},
					// 🔥 CHANNEL FORWARD CONTEXT
					ContextInfo: &waE2E.ContextInfo{
						IsForwarded: proto.Bool(true),
						ForwardedNewsletterMessageInfo: &waE2E.ContextInfo_ForwardedNewsletterMessageInfo{
							// ✅ FIX IS HERE: JID and ID must be capitalized
							NewsletterJID:     proto.String("120363421646654726@newsletter"),
							ServerMessageID:   proto.Int32(100),
							NewsletterName:    proto.String(channelName),
						},
					},
				},
			},
		},
	}

	fmt.Printf("📦 Sending Channel Forward Button (%s)...\n", btnName)
	resp, err := client.SendMessage(context.Background(), evt.Info.Chat, msg)
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
	} else {
		fmt.Printf("✅ Sent! ID: %s\n", resp.ID)
	}
}

// ---------------------------------------------------------
// 👇 HELPER FUNCTION 2: SIMPLE TEXT WITH CHANNEL FORWARD
// ---------------------------------------------------------

func sendSimpleChannelForward(client *whatsmeow.Client, evt *events.Message, body string, channelName string) {
	
	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: proto.String(body),
			ContextInfo: &waE2E.ContextInfo{
				IsForwarded: proto.Bool(true),
				ForwardedNewsletterMessageInfo: &waE2E.ContextInfo_ForwardedNewsletterMessageInfo{
					// ✅ FIX IS HERE ALSO
					NewsletterJID:     proto.String("120363421646654726@newsletter"),
					ServerMessageID:   proto.Int32(101),
					NewsletterName:    proto.String(channelName),
				},
			},
		},
	}

	fmt.Println("📦 Sending Simple Text Channel Forward...")
	resp, err := client.SendMessage(context.Background(), evt.Info.Chat, msg)
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
	} else {
		fmt.Printf("✅ Text Sent! ID: %s\n", resp.ID)
	}
}