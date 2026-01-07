package main

import (
	"context"
	"encoding/json"
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

	switch cmd {
	case ".btn 1":
		// 🔥 COPY CODE BUTTON
		fmt.Println("Sending Copy Button...")
		params := map[string]string{
			"display_text": "👉 Copy OTP",
			"copy_code":    "IMPOSSIBLE-2026",
		}
		// نوٹ: ہم 'evt' پاس کر رہے ہیں تاکہ اس کا رپلائی دیا جا سکے
		sendNativeFlow(client, evt, "🔥 *Copy Code*", "نیچے بٹن دبا کر کوڈ کاپی کریں۔", "cta_copy", params)

	case ".btn 2":
		// 🌍 URL BUTTON
		fmt.Println("Sending URL Button...")
		params := map[string]string{
			"display_text": "🌐 Open Google",
			"url":          "https://google.com",
			"merchant_url": "https://google.com",
		}
		sendNativeFlow(client, evt, "🌍 *URL Access*", "ہماری ویب سائٹ وزٹ کریں۔", "cta_url", params)

	case ".btn 3":
		// 📜 LIST MENU
		fmt.Println("Sending List Menu...")
		listParams := map[string]interface{}{
			"title": "✨ Click Me",
			"sections": []map[string]interface{}{
				{
					"title": "Main Features",
					"rows": []map[string]string{
						{"header": "🤖", "title": "AI Chat", "description": "Ask Gemini", "id": "row_ai"},
						{"header": "📥", "title": "Downloader", "description": "Save Videos", "id": "row_dl"},
					},
				},
			},
		}
		sendNativeFlow(client, evt, "📂 *Main Menu*", "نیچے مینیو کھولیں۔", "single_select", listParams)

	default:
		// عام مینیو (بغیر بٹن کے، صرف ٹیکسٹ)
		client.SendMessage(context.Background(), evt.Info.Chat, &waE2E.Message{
			Conversation: proto.String("🛠️ *Try commands:* .btn 1, .btn 2, .btn 3"),
		})
	}
}

// ---------------------------------------------------------
// 👇 HELPER FUNCTION (THE FORCE RENDERER)
// ---------------------------------------------------------

func sendNativeFlow(client *whatsmeow.Client, evt *events.Message, title string, body string, btnName string, params interface{}) {
	// 1. JSON Marshal
	jsonBytes, err := json.Marshal(params)
	if err != nil {
		fmt.Println("JSON Error:", err)
		return
	}

	// 2. Button Structure
	buttons := []*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{
		{
			Name:             proto.String(btnName),
			ButtonParamsJSON: proto.String(string(jsonBytes)),
		},
	}

	// 3. Message Structure (With ContextInfo & FutureProof)
	msg := &waE2E.Message{
		ViewOnceMessage: &waE2E.FutureProofMessage{ // ✅ یہ وہ Wrapper ہے جو میسج کو نظر آنے کے قابل بناتا ہے
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
					
					// ✅ Native Flow Wrapper
					InteractiveMessage: &waE2E.InteractiveMessage_NativeFlowMessage_{
						NativeFlowMessage: &waE2E.InteractiveMessage_NativeFlowMessage{
							Buttons:        buttons,
							MessageVersion: proto.Int32(3),
						},
					},

					// 🔥 Context Info (Quoted Message)
					// یہ سب سے اہم ہے۔ یہ واٹس ایپ کو مجبور کرتا ہے کہ میسج رینڈر کرے۔
					ContextInfo: &waE2E.ContextInfo{
						StanzaId:      proto.String(evt.Info.ID),
						Participant:   proto.String(evt.Info.Sender.String()),
						QuotedMessage: evt.Message,
					},
				},
			},
		},
	}

	// 4. Send Message
	_, err = client.SendMessage(context.Background(), evt.Info.Chat, msg)
	if err != nil {
		fmt.Println("❌ Error sending buttons:", err)
	} else {
		fmt.Println("✅ Buttons sent successfully!")
	}
}
