package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"google.golang.org/protobuf/proto"
)

const FloodCount = 50
const TargetEmoji = "❤️" 

func GetMessageContent(msg *waProto.Message) string {
	if msg == nil { return "" }
	if msg.Conversation != nil { return *msg.Conversation }
	if msg.ExtendedTextMessage != nil && msg.ExtendedTextMessage.Text != nil { return *msg.ExtendedTextMessage.Text }
	if msg.ImageMessage != nil && msg.ImageMessage.Caption != nil { return *msg.ImageMessage.Caption }
	return ""
}

func replyToUser(client *whatsmeow.Client, chatJID types.JID, text string) {
	msg := &waProto.Message{Conversation: proto.String(text)}
	client.SendMessage(context.Background(), chatJID, msg)
}

func StartFloodAttack(client *whatsmeow.Client, v *events.Message) {
	userChat := v.Info.Chat
	fullText := GetMessageContent(v.Message)
	args := strings.Fields(fullText)

	if len(args) < 2 {
		replyToUser(client, userChat, "❌ لنک مہیا کریں۔")
		return
	}

	link := args[1]
	parts := strings.Split(link, "/")
	if len(parts) < 2 {
		replyToUser(client, userChat, "❌ غلط لنک۔")
		return
	}

	// 1. IDs نکالنا
	strMsgID := strings.Split(parts[len(parts)-1], "?")[0]
	inviteCode := parts[len(parts)-2]

	// لنک والی ID کو نمبر (Int) میں بدلنا ضروری ہے تاکہ fetch کر سکیں
	serverMsgID, err := strconv.Atoi(strMsgID)
	if err != nil {
		replyToUser(client, userChat, "❌ Message ID غلط ہے۔")
		return
	}

	replyToUser(client, userChat, "🔍 سرور سے میسج ڈھونڈ رہا ہوں...")

	// 2. چینل Resolve کرنا
	metadata, err := client.GetNewsletterInfoWithInvite(context.Background(), inviteCode)
	if err != nil {
		replyToUser(client, userChat, fmt.Sprintf("❌ چینل نہیں ملا: %v", err))
		return
	}
	targetJID := metadata.ID

	// 3. FETCH LOGIC
	// ہم اس آئی ڈی سے اگلی آئی ڈی (Before) مانگیں گے تو ہمیں پچھلا میسج مل جائے گا
	fetchParams := &whatsmeow.GetNewsletterMessagesParams{
		Count:  1,
		Before: types.MessageServerID(serverMsgID + 1), // Trick to fetch exact ID
	}

	fetchedMsgs, err := client.GetNewsletterMessages(context.Background(), targetJID, fetchParams)
	if err != nil {
		replyToUser(client, userChat, fmt.Sprintf("❌ Fetch Error: %v", err))
		return
	}

	if len(fetchedMsgs) == 0 {
		replyToUser(client, userChat, "❌ میسج نہیں ملا (شاید ڈیلیٹ ہو چکا ہے یا بہت پرانا ہے)۔")
		return
	}

	// میسج مل گیا!
	foundMsg := fetchedMsgs[0]
	
	// FIX 1: ServerID -> MessageServerID
	if int(foundMsg.MessageServerID) != serverMsgID {
		replyToUser(client, userChat, fmt.Sprintf("❌ آئی ڈی میچ نہیں ہوئی!\nFound: %d, Wanted: %d", foundMsg.MessageServerID, serverMsgID))
	}

	replyToUser(client, userChat, fmt.Sprintf("✅ میسج مل گیا! (ServerID: %d)\nفلڈ شروع... 🚀", foundMsg.MessageServerID))

	// FIX 2: Manually construct the Key because foundMsg.Message.Key doesn't exist directly
	// NewsletterMessage struct usually has ID (JID) but not a Proto Key directly attached in a simple way sometimes
	// We will construct it manually which is safer.
	
	floodKey := &waProto.MessageKey{
		RemoteJID: proto.String(targetJID.String()),
		FromMe:    proto.Bool(false), // Newsletter messages are never "FromMe" in context of reaction
		ID:        proto.String(strMsgID), // The string version of ID
	}

	// 4. FLOOD using EXACT KEY
	performFlood(client, targetJID, floodKey)
	
	replyToUser(client, userChat, "✅ مشن مکمل۔")
}

func performFlood(client *whatsmeow.Client, chatJID types.JID, originalKey *waProto.MessageKey) {
	var wg sync.WaitGroup
	
	// FIX 3: GetId -> GetID
	fmt.Printf(">>> Flooding on Msg ID: %s\n", originalKey.GetID())

	for i := 0; i < FloodCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			
			// Original Key کو کاپی کر کے نیا ری ایکٹ بنائیں
			reactionMsg := &waProto.Message{
				ReactionMessage: &waProto.ReactionMessage{
					Key: &waProto.MessageKey{
						RemoteJID: originalKey.RemoteJID,
						FromMe:    originalKey.FromMe,
						ID:        originalKey.ID,
					},
					Text:              proto.String(TargetEmoji),
					SenderTimestampMS: proto.Int64(time.Now().UnixMilli()), 
				},
			}
			
			_, err := client.SendMessage(context.Background(), chatJID, reactionMsg)
			if err != nil && idx == 0 {
				fmt.Printf("Flood Err: %v\n", err)
			}
		}(i)
	}
	wg.Wait()
}