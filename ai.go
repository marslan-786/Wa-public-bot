package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types/events"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"google.golang.org/protobuf/proto"
)

// 💾 AI کی یادداشت کا اسٹرکچر
type AISession struct {
	History     string `json:"history"`       // پرانی بات چیت
	LastMsgID   string `json:"last_msg_id"`   // آخری AI میسج کی ID
	LastUpdated int64  `json:"last_updated"`  // کب بات ہوئی تھی
}

// 🧠 1. MAIN AI FUNCTION (Command Handler)
func handleAI(client *whatsmeow.Client, v *events.Message, query string, cmd string) {
	if query == "" {
		replyMessage(client, v, "⚠️ Please provide a prompt.")
		return
	}
	
	// چیٹ شروع کریں (نئی یا پرانی)
	processAIConversation(client, v, query, cmd, false)
}

// 🧠 2. REPLY HANDLER (Process Message میں استعمال ہوگا)
func handleAIReply(client *whatsmeow.Client, v *events.Message) bool {
	// 1. چیک کریں کہ کیا یہ رپلائی ہے؟
	ext := v.Message.GetExtendedTextMessage()
	if ext == nil || ext.ContextInfo == nil || ext.ContextInfo.StanzaID == nil {
		return false
	}
	
	replyToID := ext.ContextInfo.GetStanzaID()
	senderID := v.Info.Sender.ToNonAD().String()

	// 2. Redis سے چیک کریں کہ کیا یہ رپلائی AI کے میسج پر ہے؟
	if rdb != nil {
		key := "ai_session:" + senderID
		val, err := rdb.Get(context.Background(), key).Result()
		if err == nil {
			var session AISession
			json.Unmarshal([]byte(val), &session)

			// 🎯 اگر یوزر نے اسی میسج کو رپلائی کیا جو AI نے بھیجا تھا
			if session.LastMsgID == replyToID {
				// میسج کا ٹیکسٹ نکالیں
				userMsg := v.Message.GetConversation()
				if userMsg == "" {
					userMsg = v.Message.GetExtendedTextMessage().GetText()
				}
				
				// بات چیت آگے بڑھائیں
				processAIConversation(client, v, userMsg, "ai", true)
				return true // بتا دیں کہ یہ ہینڈل ہو گیا ہے
			}
		}
	}
	return false
}

// ⚙️ INTERNAL LOGIC (Common for Command & Reply)
func processAIConversation(client *whatsmeow.Client, v *events.Message, query string, cmd string, isReply bool) {
	react(client, v.Info.Chat, v.Info.ID, "🧠")

	senderID := v.Info.Sender.ToNonAD().String()
	var history string = ""
	
	// --- REDIS: پرانی چیٹ لوڈ کریں ---
	if rdb != nil {
		key := "ai_session:" + senderID
		val, err := rdb.Get(context.Background(), key).Result()
		if err == nil {
			var session AISession
			json.Unmarshal([]byte(val), &session)
			
			// اگر سیشن 30 منٹ سے پرانا ہو تو نیا شروع کریں
			if time.Now().Unix() - session.LastUpdated < 1800 {
				history = session.History
			}
		}
	}

	// 🕵️ Prompt Engineering
	aiName := "Impossible AI"
	if strings.ToLower(cmd) == "gpt" { aiName = "GPT-4" }
	
	// ہسٹری کو لمٹ کریں (تاکہ URL بہت لمبا نہ ہو جائے)
	if len(history) > 2000 {
		history = history[len(history)-2000:] // پچھلے 2000 حروف رکھیں
	}

	// سسٹم پرومپٹ + ہسٹری + نیا سوال
	fullPrompt := fmt.Sprintf(
		"System: You are %s. You are helpful, funny and precise. Respond in user's language.\n%s\nUser: %s\nAI:",
		aiName, history, query)

	// 🚀 ماڈلز کی لسٹ
	models := []string{"openai", "mistral", "karma"}
	var finalResponse string
	success := false

	for _, model := range models {
		// URL میں بھیجنے کے لیے انکوڈنگ
		apiUrl := fmt.Sprintf("https://text.pollinations.ai/%s?model=%s", 
			url.QueryEscape(fullPrompt), model)

		clientHttp := http.Client{Timeout: 30 * time.Second}
		resp, err := clientHttp.Get(apiUrl)
		if err != nil { continue }
		
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		res := string(body)

		if strings.HasPrefix(res, "{") && strings.Contains(res, "error") {
			continue 
		}

		finalResponse = res
		success = true
		break
	}

	if !success {
		replyMessage(client, v, "🤖 Brain Overload! Try again.")
		return
	}

	// ✅ جواب بھیجیں اور ID نوٹ کریں
	respPtr, err := client.SendMessage(context.Background(), v.Info.Chat, &waProto.Message{
		ExtendedTextMessage: &waProto.ExtendedTextMessage{
			Text: proto.String(finalResponse),
			ContextInfo: &waProto.ContextInfo{
				StanzaID:      proto.String(v.Info.ID),
				Participant:   proto.String(v.Info.Sender.String()),
				QuotedMessage: v.Message,
			},
		},
	})

	if err == nil {
		// --- REDIS: نیا ڈیٹا محفوظ کریں ---
		if rdb != nil {
			newHistory := fmt.Sprintf("%s\nUser: %s\nAI: %s", history, query, finalResponse)
			
			newSession := AISession{
				History:     newHistory,
				LastMsgID:   respPtr.ID, // ✅ یہاں ہم AI کے میسج کی ID سیو کر رہے ہیں
				LastUpdated: time.Now().Unix(),
			}
			
			jsonData, _ := json.Marshal(newSession)
			// 30 منٹ کا ٹائم آؤٹ (اس کے بعد چیٹ بھول جائے گا)
			rdb.Set(context.Background(), "ai_session:"+senderID, jsonData, 30*time.Minute)
		}
		
		// اگر یہ رپلائی نہیں تھا تو گرین ٹک، ورنہ خاموشی
		if !isReply {
			react(client, v.Info.Chat, v.Info.ID, "✅")
		}
	}
}
