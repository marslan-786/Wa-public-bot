package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
	"encoding/json"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"google.golang.org/protobuf/proto"
	"github.com/redis/go-redis/v9"
)

// 🛡️ سیٹنگز کا ڈھانچہ (Structure)
// اس میں تم مزید چیزیں بھی ڈال سکتے ہو جیسے AntiLink، Welcome وغیرہ
type BotSettings struct {
	Prefix     string `json:"prefix"`
	SelfMode   bool   `json:"self_mode"`
	AutoStatus bool   `json:"auto_status"`
	OnlyGroup  bool   `json:"only_group"`
	 
}

// 💾 1. تمام سیٹنگز ریڈیس میں محفوظ کرنا
func SaveAllSettings(rdb *redis.Client, botID string, settings BotSettings) {
	// ڈیٹا کو JSON میں بدلیں
	data, err := json.Marshal(settings)
	if err != nil {
		fmt.Println("❌ [REDIS] JSON encoding error:", err)
		return
	}

	// ریڈیس میں بوٹ کی آئی ڈی کے نام سے سیو کریں
	key := fmt.Sprintf("settings:%s", botID)
	err = rdb.Set(ctx, key, data, 0).Err() // 0 کا مطلب ہے کبھی ڈیلیٹ نہ ہو
	if err != nil {
		fmt.Println("❌ [REDIS] Save error:", err)
	} else {
		fmt.Printf("✅ [SAVED] Settings for %s stored in Redis\n", botID)
	}
}

// 📥 2. ریڈیس سے سیٹنگز واپس لوڈ کرنا
func LoadAllSettings(rdb *redis.Client, botID string) BotSettings {
	key := fmt.Sprintf("settings:%s", botID)
	val, err := rdb.Get(ctx, key).Result()

	var settings BotSettings
	if err == redis.Nil {
		// اگر پہلے سے کوئی سیٹنگ نہیں ہے تو ڈیفالٹ سیٹ کریں
		fmt.Println("ℹ️ [REDIS] No settings found, using defaults.")
		return BotSettings{Prefix: ".", SelfMode: false, AutoStatus: true}
	} else if err != nil {
		fmt.Println("❌ [REDIS] Load error:", err)
		return BotSettings{Prefix: "."}
	}

	// JSON سے واپس اسٹرکچر میں بدلیں
	err = json.Unmarshal([]byte(val), &settings)
	if err != nil {
		fmt.Println("❌ [REDIS] JSON decoding error:", err)
	}
	
	fmt.Printf("🚀 [LOADED] Settings for %s synced from Redis\n", botID)
	return settings
}

// 🛡️ گروپ سیکیورٹی سیٹنگز کا ڈھانچہ
type GroupSecurity struct {
	AntiLink   bool `json:"anti_link"`
	AllowAdmin bool `json:"allow_admin"` // جو آپ اسٹیج 1 میں پوچھ رہے ہیں
}

// 💾 گروپ سیٹنگ سیو کرنا (Group Specific)
func SaveGroupSecurity(rdb *redis.Client, botLID string, groupID string, data GroupSecurity) {
	key := fmt.Sprintf("sec:%s:%s", botLID, groupID)
	payload, _ := json.Marshal(data)
	
	err := rdb.Set(ctx, key, payload, 0).Err()
	if err != nil {
		fmt.Printf("❌ [REDIS] Save Error for Group %s: %v\n", groupID, err)
	}
}

// 📥 گروپ سیٹنگ لوڈ کرنا (Group Specific)
func LoadGroupSecurity(rdb *redis.Client, botLID string, groupID string) GroupSecurity {
	key := fmt.Sprintf("sec:%s:%s", botLID, groupID)
	val, err := rdb.Get(ctx, key).Result()
	
	var data GroupSecurity
	if err != nil {
		// اگر کوئی سیٹنگ نہیں ملی تو ڈیفالٹ (False) واپس کریں
		return GroupSecurity{AntiLink: false, AllowAdmin: false}
	}
	
	json.Unmarshal([]byte(val), &data)
	return data
}

// فرض کریں یوزر نے 'antilink' آن کرنے کا فیصلہ کر لیا ہے
func finalizeSecurity(client *whatsmeow.Client, senderLID string, choice string) {
	state := setupMap[senderLID]
	if state == nil { return }

	allowAdmin := (choice == "1") // اگر یوزر نے 1 دبایا تو ایڈمن الاؤ ہیں
	
	// سیٹنگز تیار کریں
	newConfig := GroupSecurity{
		AntiLink:   true, // کیونکہ وہ اینٹی لنک کا سیٹ اپ کر رہا تھا
		AllowAdmin: allowAdmin,
	}

	// 💾 ریڈیس میں اس گروپ کے لیے مخصوص سیو کریں
	SaveGroupSecurity(rdb, state.BotLID, state.GroupID, newConfig)
	
	// میپ سے ڈیلیٹ کر دیں
	delete(setupMap, senderLID)
}
// ==================== سیکورٹی سسٹم ====================
func checkSecurity(client *whatsmeow.Client, v *events.Message) {
	// ✅ 1. Bot ID نکالیں
	rawBotID := client.Store.ID.User
	botID := getCleanID(rawBotID)

	if !v.Info.IsGroup {
		return
	}

	// ✅ 2. Settings حاصل کرتے وقت botID پاس کریں
	s := getGroupSettings(botID, v.Info.Chat.String())
	
	if s.Mode == "private" {
		return
	}

	// ✅ Anti-link check
	if s.Antilink && containsLink(getText(v.Message)) {
		// نوٹ: takeSecurityAction کو بھی botID پاس کیا ہے تاکہ وہ Save کر سکے
		takeSecurityAction(client, v, s, s.AntilinkAction, "Link detected", botID)
		return
	}

	// Anti-picture check
	if s.AntiPic && v.Message.ImageMessage != nil {
		takeSecurityAction(client, v, s, "delete", "Image not allowed", botID)
		return
	}

	// Anti-video check
	if s.AntiVideo && v.Message.VideoMessage != nil {
		takeSecurityAction(client, v, s, "delete", "Video not allowed", botID)
		return
	}

	// Anti-sticker check
	if s.AntiSticker && v.Message.StickerMessage != nil {
		takeSecurityAction(client, v, s, "delete", "Sticker not allowed", botID)
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

// ✅ فنکشن میں botID کا اضافہ کیا گیا ہے
func takeSecurityAction(client *whatsmeow.Client, v *events.Message, s *GroupSettings, action, reason string, botID string) {
	switch action {
	case "delete":
		// ✅ Delete for everyone
		_, err := client.SendMessage(context.Background(), v.Info.Chat, client.BuildRevoke(v.Info.Chat, v.Info.Sender, v.Info.ID))
		if err != nil {
			// log.Printf("❌ Delete failed: %v", err) // Optional Log
			return
		}

		msg := fmt.Sprintf(`╔════════════════╗
║ 🚫 DELETED
╠════════════════╣
║ Reason: %s
║ User: @%s
╚════════════════╝`, reason, v.Info.Sender.User)
		
		senderStr := v.Info.Sender.String()
		client.SendMessage(context.Background(), v.Info.Chat, &waProto.Message{
			ExtendedTextMessage: &waProto.ExtendedTextMessage{
				Text: proto.String(msg),
				ContextInfo: &waProto.ContextInfo{
					MentionedJID: []string{senderStr},
					StanzaID:     proto.String(v.Info.ID),
					Participant:  proto.String(senderStr),
				},
			},
		})

	case "deletekick":
		// 1. Delete
		client.SendMessage(context.Background(), v.Info.Chat, client.BuildRevoke(v.Info.Chat, v.Info.Sender, v.Info.ID))

		// 2. Kick
		_, err := client.UpdateGroupParticipants(context.Background(), v.Info.Chat,
			[]types.JID{v.Info.Sender}, whatsmeow.ParticipantChangeRemove)
		
		if err != nil {
			replyMessage(client, v, "⚠️ Failed to Kick (Need Admin)")
			return
		}
		
		msg := fmt.Sprintf(`╔════════════════╗
║ 👢 KICKED
╠════════════════╣
║ Reason: %s
║ User: @%s
║ Action: Delete+Kick
╚════════════════╝`, reason, v.Info.Sender.User)
		
		senderStr := v.Info.Sender.String()
		client.SendMessage(context.Background(), v.Info.Chat, &waProto.Message{
			ExtendedTextMessage: &waProto.ExtendedTextMessage{
				Text: proto.String(msg),
				ContextInfo: &waProto.ContextInfo{
					MentionedJID: []string{senderStr},
				},
			},
		})

	case "deletewarn":
		// 1. Delete
		client.SendMessage(context.Background(), v.Info.Chat, client.BuildRevoke(v.Info.Chat, v.Info.Sender, v.Info.ID))

		// 2. Update Warnings
		senderKey := v.Info.Sender.String()
		if s.Warnings == nil {
			s.Warnings = make(map[string]int)
		}
		s.Warnings[senderKey]++
		warnCount := s.Warnings[senderKey]

		if warnCount >= 3 {
			// Kick after 3 warnings
			_, err := client.UpdateGroupParticipants(context.Background(), v.Info.Chat,
				[]types.JID{v.Info.Sender}, whatsmeow.ParticipantChangeRemove)
			
			if err != nil {
				replyMessage(client, v, "⚠️ Failed to Kick (Need Admin)")
			} else {
				delete(s.Warnings, senderKey) // Reset warnings
				
				msg := fmt.Sprintf(`╔════════════════╗
║ 🚫 KICKED
╠════════════════╣
║ User: @%s
║ Warning: 3/3
║ Reason: %s
╚════════════════╝`, v.Info.Sender.User, reason)
				
				senderStr := v.Info.Sender.String()
				client.SendMessage(context.Background(), v.Info.Chat, &waProto.Message{
					ExtendedTextMessage: &waProto.ExtendedTextMessage{
						Text: proto.String(msg),
						ContextInfo: &waProto.ContextInfo{MentionedJID: []string{senderStr}},
					},
				})
			}
		} else {
			// Send Warning Message
			msg := fmt.Sprintf(`╔════════════════╗
║ ⚠️ WARNING
╠════════════════╣
║ User: @%s
║ Count: %d/3
║ Reason: %s
╚════════════════╝`, v.Info.Sender.User, warnCount, reason)
			
			senderStr := v.Info.Sender.String()
			client.SendMessage(context.Background(), v.Info.Chat, &waProto.Message{
				ExtendedTextMessage: &waProto.ExtendedTextMessage{
					Text: proto.String(msg),
					ContextInfo: &waProto.ContextInfo{
						MentionedJID: []string{senderStr},
						StanzaID:     proto.String(v.Info.ID),
						Participant:  proto.String(senderStr),
					},
				},
			})
		}

		// ✅ FIX: Save with BotID
		saveGroupSettings(botID, s)
	}
}
// مثال کے طور پر
func onResponse(client *whatsmeow.Client, v *events.Message, choice string) {
	senderID := v.Info.Sender.String()
	state, exists := setupMap[senderID]

	// 1. کیا یہ بندہ سیٹ اپ موڈ میں ہے؟
	if !exists { return }

	// 2. کیا اس نے میسج کو ریپلائی (Quote) کیا ہے؟
	if v.Message.GetExtendedTextMessage().GetContextInfo() == nil {
		return // اگر ریپلائی نہیں ہے تو خاموش رہے
	}

	// 3. کیا ریپلائی اسی بوٹ کے میسج کو کیا گیا ہے؟
	quotedID := v.Message.ExtendedTextMessage.ContextInfo.GetStanzaID() // ✅ Fixed: ID caps mein
	if quotedID != state.BotMsgID {
		return // اگر کسی اور کے میسج کو ریپلائی کیا تو اگنور کریں
	}

	// 4. اگر سب ٹھیک ہے تو ریڈیس میں سیو کریں
	key := fmt.Sprintf("group:sec:%s:%s:%s", state.BotLID, state.GroupID, state.Type)
	rdb.Set(context.Background(), key, choice, 0)

	// اگلا مینیو دکھائیں یا ختم کریں
	replyMessage(client, v, "✅ Setting Saved Successfully!")
	delete(setupMap, senderID)
}

func startSecuritySetup(client *whatsmeow.Client, v *events.Message, secType string) {
	// 1️⃣ گروپ چیک
	if !v.Info.IsGroup {
		replyMessage(client, v, "╔════════════════╗\n║ ❌ GROUP ONLY\n╚════════════════╝")
		return
	}

	// 2️⃣ ایڈمن چیک
	isAdmin := false
	groupInfo, _ := client.GetGroupInfo(context.Background(), v.Info.Chat)
	if groupInfo != nil {
		for _, p := range groupInfo.Participants {
			if p.JID.User == v.Info.Sender.User && (p.IsAdmin || p.IsSuperAdmin) {
				isAdmin = true; break
			}
		}
	}
	if !isAdmin && !isOwner(client, v.Info.Sender) {
		replyMessage(client, v, "╔════════════════╗\n║ 👮 ADMIN ONLY\n╚════════════════╝")
		return
	}

	// 🛠️ یوزر کی LID کلین کریں (صرف نمبر نکالیں)
	// v.Info.Sender.User میں عام طور پر LID کا نمبر ہی ہوتا ہے
	cleanSenderLID := v.Info.Sender.User 
	groupID := v.Info.Chat.String()
	botUniqueLID := getBotLIDFromDB(client) // بوٹ کی اپنی پہچان

	msgText := fmt.Sprintf(`╔════════════════╗
║ 🛡️ %s (1/2)
╠════════════════╣
║ Allow Admins?
║ 1️⃣ YES | 2️⃣ NO
╚════════════════╝`, strings.ToUpper(secType))

	// کارڈ بھیجیں
	resp, err := client.SendMessage(context.Background(), v.Info.Chat, &waProto.Message{
		ExtendedTextMessage: &waProto.ExtendedTextMessage{Text: proto.String(msgText)},
	})

	if err != nil {
		fmt.Printf("❌ ERROR: %v\n", err)
		return 
	}

	// 🔑 میسج آئی ڈی کو ہی 'Key' بنائیں
	mapKey := resp.ID 

	fmt.Printf("\n🔥 [LOG] Card Sent | ID: %s | User LID: %s\n", resp.ID, cleanSenderLID)

	// 💾 سیشن محفوظ کریں
	setupMap[mapKey] = &SetupState{
		Type:     secType,
		Stage:    1,
		GroupID:  groupID,
		User:     cleanSenderLID, // محفوظ شدہ کلین LID
		BotLID:   botUniqueLID,
		BotMsgID: resp.ID,
	}

	go func() {
		time.Sleep(2 * time.Minute)
		delete(setupMap, mapKey)
	}()
}

func handleSetupResponse(client *whatsmeow.Client, v *events.Message) {
	// 🛑 ریپلائی چیک
	extMsg := v.Message.GetExtendedTextMessage()
	if extMsg == nil || extMsg.ContextInfo == nil {
		return
	}

	quotedID := extMsg.ContextInfo.GetStanzaID()
	incomingLID := v.Info.Sender.User // واٹس ایپ ہمیشہ LID بھیجتا ہے

	// ✅ FIX 1: موجودہ بوٹ کی کلین آئی ڈی نکالیں (سیٹنگز کے لیے ضروری ہے)
	rawBotID := client.Store.ID.User
	botID := getCleanID(rawBotID)

	// 1. ڈیٹا تلاش کریں
	state, exists := setupMap[quotedID]
	if !exists {
		return
	}

	// 2. بوٹ میچنگ (صرف وہی بوٹ جواب دے جس کا یہ سیشن ہے)
	if state.BotLID != botID {
		return
	}

	// 3. یوزر میچنگ
	fmt.Printf("🔍 [COMPARING] StoredLID: %s | IncomingLID: %s\n", state.User, incomingLID)

	if state.User != incomingLID {
		fmt.Println("🚫 [REJECTED] User LID mismatch.")
		// return // ٹیسٹنگ کے دوران اسے کمنٹ کر سکتے ہیں اگر LID کا مسئلہ ہو
	}

	fmt.Printf("✅ [MATCH] Stage %d logic starting...\n", state.Stage)

	txt := strings.TrimSpace(getText(v.Message))

	// ✅ FIX 2: سیٹنگز نکالتے وقت botID پاس کریں
	s := getGroupSettings(botID, state.GroupID)

	// --- اسٹیج 1: ایڈمن بائی پاس ---
	if state.Stage == 1 {
		if txt == "1" {
			s.AntilinkAdmin = true
		} else if txt == "2" {
			s.AntilinkAdmin = false
		} else {
			return
		}

		delete(setupMap, quotedID) // پرانا کارڈ ہٹائیں

		state.Stage = 2
		nextMsg := fmt.Sprintf(`╔════════════════╗
║ ⚡ %s (2/2)
╠════════════════╣
║ 1️⃣ DELETE ONLY
║ 2️⃣ DELETE + KICK
║ 3️⃣ DELETE + WARN
╚════════════════╝`, strings.ToUpper(state.Type))

		resp, _ := client.SendMessage(context.Background(), v.Info.Chat, &waProto.Message{
			ExtendedTextMessage: &waProto.ExtendedTextMessage{Text: proto.String(nextMsg)},
		})

		state.BotMsgID = resp.ID
		setupMap[resp.ID] = state // نئی میسج آئی ڈی سیو کریں
		fmt.Printf("⏭️ [NEXT] Stage 2 sent. New Wait ID: %s\n", resp.ID)
		return
	}

	// --- اسٹیج 2: ایکشن سیٹ اپ ---
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
			return
		}

		applySecurityFinal(s, state.Type, true)

		// ✅ FIX 3: سیو کرتے وقت بھی botID پاس کریں (تاکہ Redis میں صحیح جگہ سیو ہو)
		saveGroupSettings(botID, s)
		
		delete(setupMap, quotedID) // سیشن ختم

		adminBypass := "YES ✅"
		if !s.AntilinkAdmin {
			adminBypass = "NO ❌"
		}
		finalMsg := fmt.Sprintf(`╔════════════════╗
║ ✅ %s ENABLED
╠════════════════╣
║ Admin Bypass: %s
║ Action: %s
╚════════════════╝`, strings.ToUpper(state.Type), adminBypass, actionText)

		replyMessage(client, v, finalMsg)
		fmt.Printf("🏁 [COMPLETE] Setup Success for %s\n", state.Type)
	}
}

// ہیلپر
func applySecurityFinal(s *GroupSettings, t string, val bool) {
	switch t {
	case "antilink": s.Antilink = val
	case "antipic": s.AntiPic = val
	case "antivideo": s.AntiVideo = val
	case "antisticker": s.AntiSticker = val
	}
}

// ہیلپر فنکشن ایڈمن چیک کے لیے
func participantIsAdmin(p types.GroupParticipant) bool {
	return p.IsAdmin || p.IsSuperAdmin
}

func handleGroupEvents(client *whatsmeow.Client, evt interface{}) {
	switch v := evt.(type) {
	case *events.GroupInfo:
        // ⚡ اسے الگ تھریڈ میں پھینک دیں تاکہ مین بوٹ فری رہے
		go handleGroupInfoChange(client, v)
	}
}

func handleGroupInfoChange(client *whatsmeow.Client, v *events.GroupInfo) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("⚠️ Panic: %v\n", r)
		}
	}()

	if v.JID.IsEmpty() { return }
	chatID := v.JID.String()

	// ✅ 1. Bot ID نکالیں
	rawBotID := client.Store.ID.User
	botID := getCleanID(rawBotID)

	// ✅ 2. اب botID پاس کریں
	settings := getGroupSettings(botID, chatID)
	
	if !settings.Welcome { return }

	// 🛡️ ANTI-SPAM FILTER
	if RestrictedGroups[chatID] {
		if !AuthorizedBots[botID] {
			return 
		}
	}

	// ... (باقی ویلکم لاجک) ...
	// =========================================================

    // ⚡ 4. Event Processing (Join, Leave, Promote, Demote)
    
	// ✅ کک یا لیو (Leave/Kick)
	if v.Leave != nil && len(v.Leave) > 0 {
		for _, left := range v.Leave {
			sender := v.Sender 
			leftStr := left.String()
            // نام نکالنے کی کوشش (Optional)
            userNum := strings.Split(left.User, "@")[0]

			if sender.User == left.User {
                // خود لیفٹ ہوا
				msg := fmt.Sprintf(`╔════════════════╗
║ 👋 GOODBYE
╠════════════════╣
║ 👤 User: @%s
║ 📉 Status: Left
╚════════════════╝`, userNum)

				client.SendMessage(context.Background(), v.JID, &waProto.Message{
					ExtendedTextMessage: &waProto.ExtendedTextMessage{
						Text: proto.String(msg),
						ContextInfo: &waProto.ContextInfo{
							MentionedJID: []string{leftStr},
						},
					},
				})
			} else {
                // کک کیا گیا (By Admin)
				msg := fmt.Sprintf(`╔════════════════╗
║ 👢 KICKED
╠════════════════╣
║ 👤 User: @%s
║ 👮 By: @%s
╚════════════════╝`, userNum, sender.User)

				client.SendMessage(context.Background(), v.JID, &waProto.Message{
					ExtendedTextMessage: &waProto.ExtendedTextMessage{
						Text: proto.String(msg),
						ContextInfo: &waProto.ContextInfo{
							MentionedJID: []string{leftStr, sender.String()},
						},
					},
				})
			}
            time.Sleep(500 * time.Millisecond) // چھوٹا سا وقفہ تاکہ واٹس ایپ بین نہ کرے
		}
	}

	// ✅ Promote event
	if v.Promote != nil && len(v.Promote) > 0 {
		for _, promoted := range v.Promote {
			msg := fmt.Sprintf(`╔════════════════╗
║ 👑 PROMOTED
╠════════════════╣
║ 👤 User: @%s
║ 🎉 New Admin!
╚════════════════╝`, promoted.User)

			client.SendMessage(context.Background(), v.JID, &waProto.Message{
				ExtendedTextMessage: &waProto.ExtendedTextMessage{
					Text: proto.String(msg),
					ContextInfo: &waProto.ContextInfo{
						MentionedJID: []string{promoted.String()},
					},
				},
			})
		}
	}

	// ✅ Demote event
	if v.Demote != nil && len(v.Demote) > 0 {
		for _, demoted := range v.Demote {
			msg := fmt.Sprintf(`╔════════════════╗
║ 👤 DEMOTED
╠════════════════╣
║ 👤 User: @%s
║ 📉 Admin Removed
╚════════════════╝`, demoted.User)

			client.SendMessage(context.Background(), v.JID, &waProto.Message{
				ExtendedTextMessage: &waProto.ExtendedTextMessage{
					Text: proto.String(msg),
					ContextInfo: &waProto.ContextInfo{
						MentionedJID: []string{demoted.String()},
					},
				},
			})
		}
	}

	// ✅ Join event (Welcome)
	if v.Join != nil && len(v.Join) > 0 {
		for _, joined := range v.Join {
			msg := fmt.Sprintf(`╔════════════════╗
║ 👋 WELCOME
╠════════════════╣
║ 👤 User: @%s
║ 🎉 Enjoy here!
╚════════════════╝`, joined.User)

			client.SendMessage(context.Background(), v.JID, &waProto.Message{
				ExtendedTextMessage: &waProto.ExtendedTextMessage{
					Text: proto.String(msg),
					ContextInfo: &waProto.ContextInfo{
						MentionedJID: []string{joined.String()},
					},
				},
			})
            time.Sleep(500 * time.Millisecond)
		}
	}
}