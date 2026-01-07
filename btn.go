package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E" // 🟢 Verified Path: Contains modern interactive message definitions
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

// -----------------------------------------------------------------------------
// 🎛️ MAIN SWITCH HANDLER
// -----------------------------------------------------------------------------
// This function acts as the central router for command processing.
// It inspects incoming messages for the ".btn" prefix and dispatches
// the appropriate Native Flow payload.
func HandleButtonCommands(client *whatsmeow.Client, evt *events.Message) {
	// 1. Extract the Message Text
	// We must check both 'Conversation' (simple text) and 'ExtendedTextMessage' (text with preview/context).
	// This ensures the bot works even if the user replies to another message.
	text := evt.Message.GetConversation()
	if text == "" {
		text = evt.Message.GetExtendedTextMessage().GetText()
	}

	// 2. Filter for Commands
	// Using HasPrefix allows for arguments, though currently we split exactly.
	if!strings.HasPrefix(strings.ToLower(text), ".btn") {
		return
	}

	chatJID := evt.Info.Chat
	cmd := strings.TrimSpace(strings.ToLower(text))

	// 3. Switch on Command Logic
	switch cmd {
	case ".btn 1":
		// ---------------------------------------------------------------------
		// SCENARIO 1: The "Copy Code" Button
		// ---------------------------------------------------------------------
		// Technical Note: This uses the 'cta_copy' native flow.
		// The client will render a button that, when tapped, copies 'copy_code' to clipboard.
		fmt.Println("Testing Copy Button...")
		
		params := map[string]string{
			"display_text": "👉 Copy Code",
			"copy_code":    "IMPOSSIBLE-2026",
			"id":           "btn_copy_unique_id_123", // Best practice: always include unique IDs for tracking
		}
		
		sendNativeFlow(client, chatJID, "🔥 *Copy Button Test*", "نیچے بٹن دبا کر کوڈ کاپی کریں۔", "cta_copy", params)

	case ".btn 2":
		// ---------------------------------------------------------------------
		// SCENARIO 2: The "URL Redirect" Button
		// ---------------------------------------------------------------------
		// Technical Note: This uses 'cta_url'.
		// 'merchant_url' is included as a fallback/validation field, recommended by Meta docs.
		fmt.Println("Testing URL Button...")
		
		params := map[string]string{
			"display_text": "🌐 Open Google",
			"url":          "https://google.com",
			"merchant_url": "https://google.com",
			"id":           "btn_url_unique_id_456",
		}
		
		sendNativeFlow(client, chatJID, "🌍 *URL Button Test*", "ہماری ویب سائٹ وزٹ کریں۔", "cta_url", params)

	case ".btn 3":
		// ---------------------------------------------------------------------
		// SCENARIO 3: The "List Menu" (Single Select)
		// ---------------------------------------------------------------------
		// Technical Note: 'single_select' replaces the legacy ListMessage.
		// It supports sections, rows, headers, and descriptions.
		fmt.Println("Testing List Menu...")
		
		// The JSON structure here is complex. We use map[string]interface{} to handle nested arrays.
		listParams := map[string]interface{}{
			"title": "✨ Select Option", // This text appears ON the button that opens the menu
			"sections":map[string]interface{}{
				{
					"title": "Main Features",
					"rows":map[string]string{
						{
							"header":      "🤖",
							"title":       "AI Chat",
							"description": "Chat with Gemini",
							"id":          "row_ai",
						},
						{
							"header":      "📥",
							"title":       "Downloader",
							"description": "Download Videos",
							"id":          "row_dl",
						},
					},
				},
				{
					"title": "Settings",
					"rows":map[string]string{
						{
							"header":      "⚙️",
							"title":       "Panel",
							"description": "Admin Controls",
							"id":          "row_panel",
						},
					},
				},
			},
		}
		
		sendNativeFlow(client, chatJID, "📂 *List Menu Test*", "نیچے مینیو کھولیں۔", "single_select", listParams)

	default:
		// ---------------------------------------------------------------------
		// DEFAULT: Help Menu
		// ---------------------------------------------------------------------
		// Sent as a standard text message if the command is not recognized.
		menu := "🛠️ *BUTTON TESTER MENU (Fixed)*\n\n" +
			"➤ `.btn 1` : Copy Code Button\n" +
			"➤ `.btn 2` : Open URL Button\n" +
			"➤ `.btn 3` : List Menu\n"
		
		client.SendMessage(context.Background(), chatJID, &waE2E.Message{
			Conversation: proto.String(menu),
		})
	}
}

// ---------------------------------------------------------
// 👇 HELPER FUNCTIONS (FIXED & TYPED)
// ---------------------------------------------------------

// sendNativeFlow abstracts the complexity of constructing the Protobuf hierarchy.
// It handles JSON marshaling and ensures the correct capitalization of ButtonParamsJSON.
func sendNativeFlow(client *whatsmeow.Client, jid types.JID, title string, body string, btnName string, params interface{}) {
	
	// 1. Serialize the Parameters
	// Native Flow buttons require the configuration (URL, Code, List Sections) to be a JSON string.
	jsonBytes, err := json.Marshal(params)
	if err!= nil {
		fmt.Printf("❌ JSON Marshal Error: %v\n", err)
		return
	}

	// 2. Construct the Button Element
	// 🚨 CRITICAL FIX: The field is named 'ButtonParamsJSON' (capitalized JSON).
	// The generated Go struct from 'protoc-gen-go' enforces this capitalization rule for acronyms.
	buttons :=*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{
		{
			Name:             proto.String(btnName),
			ButtonParamsJSON: proto.String(string(jsonBytes)), // FIXED: ButtonParamsJson -> ButtonParamsJSON
		},
	}

	// 3. Assemble the Interactive Message
	// We build the message from the inside out:
	// Button -> NativeFlowMessage -> InteractiveMessage -> FutureProofMessage -> ViewOnceMessage
	
	interactiveMsg := &waE2E.InteractiveMessage{
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
		
		// Native Flow Container
		InteractiveMessage: &waE2E.InteractiveMessage_NativeFlowMessage_{
			NativeFlowMessage: &waE2E.InteractiveMessage_NativeFlowMessage{
				Buttons:        buttons,
				MessageVersion: proto.Int32(3), // Version 3 is required for full Native Flow support
			},
		},
	}

	// 4. Wrap in FutureProof/ViewOnce (User's Pattern)
	// While standard messages don't strictly require ViewOnce, this structure is valid 
	// and often used to force specific UI behaviors on the client.
	msg := &waE2E.Message{
		ViewOnceMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				InteractiveMessage: interactiveMsg,
			},
		},
	}

	// 5. Transmit
	// Using context.Background() here implies a blocking send without timeout.
	// In production, consider 'context.WithTimeout(context.Background(), 10*time.Second)'
	resp, err := client.SendMessage(context.Background(), jid, msg)
	if err!= nil {
		fmt.Printf("❌ Error sending buttons: %v\n", err)
	} else {
		fmt.Printf("✅ Button Sent! Server ID: %v\n", resp.ID)
	}
}