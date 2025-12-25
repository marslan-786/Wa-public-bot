package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"google.golang.org/protobuf/proto"
)

// سیٹنگز
const FloodCount = 50
const TargetEmoji = "❤️" 

// یہ فنکشن اب آپ کو واٹس ایپ پر جواب بھی دے گا
func StartFloodAttack(client *whatsmeow.Client, v *events.Message) {
	// جس نے کمانڈ بھیجی، اسی کو جواب دینے کے لیے چیٹ آئی ڈی
	userChat := v.Info.Chat

	// 1. کمانڈ اور لنک الگ کرنا
	args := strings.Fields(v.Message.GetConversation())
	if len(args) < 2 {
		replyToUser(client, userChat, "❌ یار لنک تو دو! \nUsage: >testreact <link>")
		return
	}

	link := args[1]
	
	// اسٹیٹس اپڈیٹ 1: لنک چیکنگ
	replyToUser(client, userChat, "🔍 لنک چیک کر رہا ہوں...")

	parts := strings.Split(link, "/")
	if len(parts) < 2 {
		replyToUser(client, userChat, "❌ غلط لنک فارمیٹ ہے۔")
		return
	}

	msgID := parts[len(parts)-1]
	inviteCode := parts[len(parts)-2]

	fmt.Printf("Resolving Channel: Code=%s, MsgID=%s\n", inviteCode, msgID)

	// 2. چینل کی معلومات (Metadata)
	metadata, err := client.GetNewsletterInfoWithInvite(context.Background(), inviteCode)
	if err != nil {
		replyToUser(client, userChat, "❌ یہ چینل نہیں مل رہا، شاید لنک پرانا ہے یا غلط ہے۔")
		fmt.Printf("Failed to resolve: %v\n", err)
		return
	}

	targetJID := metadata.ID
	
	// اسٹیٹس اپڈیٹ 2: چینل مل گیا
	replyToUser(client, userChat, fmt.Sprintf("✅ ٹارگٹ مل گیا!\nID: %s\nFlood شروع کر رہا ہوں (%d Emojis)...", targetJID, FloodCount))

	// 3. فلڈ شروع کریں
	performFlood(client, targetJID, msgID)

	// اسٹیٹس اپڈیٹ 3: کام ختم
	replyToUser(client, userChat, "✅ مشن مکمل! رزلٹ چیک کرو۔")
}

func performFlood(client *whatsmeow.Client, chatJID types.JID, msgID string) {
	var wg sync.WaitGroup

	fmt.Printf(">>> Stacking %s on Msg: %s (Count: %d)\n", TargetEmoji, msgID, FloodCount)

	for i := 0; i < FloodCount; i++ {
		wg.Add(1)
		
		go func(idx int) {
			defer wg.Done()

			reactionMsg := &waProto.Message{
				ReactionMessage: &waProto.ReactionMessage{
					Key: &waProto.MessageKey{
						RemoteJID: proto.String(chatJID.String()),
						FromMe:    proto.Bool(false),
						ID:        proto.String(msgID),
					},
					Text:              proto.String(TargetEmoji),
					SenderTimestampMS: proto.Int64(time.Now().UnixMilli()), 
				},
			}

			// یہاں ہم ایرر پرنٹ نہیں کر رہے تاکہ سپیڈ تیز رہے
			client.SendMessage(context.Background(), chatJID, reactionMsg)
		}(i)
	}

	wg.Wait()
	fmt.Println(">>> Flood execution finished.")
}

// یہ چھوٹا فنکشن آپ کو میسج بھیجنے کے لیے استعمال ہوگا
func replyToUser(client *whatsmeow.Client, chatJID types.JID, text string) {
	msg := &waProto.Message{
		Conversation: proto.String(text),
	}
	client.SendMessage(context.Background(), chatJID, msg)
}
