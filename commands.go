package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"google.golang.org/protobuf/proto"
)

// --- گلوبل ویری ایبلز ---
var (
	startTime   = time.Now()
	data        BotData
	dataMutex   sync.RWMutex
	setupMap    = make(map[string]*SetupState)
	groupCache  = make(map[string]*GroupSettings)
	cacheMutex  sync.RWMutex
)

// --- مین ایونٹ ہینڈلر ---
func handler(client *whatsmeow.Client, evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		go processMessage(client, v)
	}
}

func processMessage(client *whatsmeow.Client, v *events.Message) {
	chatID := v.Info.Chat.String()
	senderID := v.Info.Sender.String()
	isGroup := v.Info.IsGroup

	// 1. سیٹ اپ فلکوئیشن
	if state, ok := setupMap[senderID]; ok && state.GroupID == chatID {
		handleSetupResponse(client, v, state)
		return
	}

	// 2. آٹو اسٹیٹس
	if chatID == "status@broadcast" {
		dataMutex.RLock()
		if data.AutoStatus {
			client.MarkRead(context.Background(), []types.MessageID{v.Info.ID}, v.Info.Timestamp, v.Info.Chat, v.Info.Sender, types.ReceiptTypeRead)
			if data.StatusReact {
				emojis := []string{"💚", "❤️", "🔥", "😍", "💯"}
				react(client, v.Info.Chat, v.Info.ID, emojis[time.Now().UnixNano()%int64(len(emojis))])
			}
		}
		dataMutex.RUnlock()
		return
	}

	// 3. آٹو ریڈ اور ری ایکٹ
	dataMutex.RLock()
	if data.AutoRead {
		client.MarkRead(context.Background(), []types.MessageID{v.Info.ID}, v.Info.Timestamp, v.Info.Chat, v.Info.Sender, types.ReceiptTypeRead)
	}
	if data.AutoReact {
		react(client, v.Info.Chat, v.Info.ID, "❤️")
	}
	dataMutex.RUnlock()

	// 4. سیکورٹی چیکس
	if isGroup {
		checkSecurity(client, v)
	}

	// 5. کمانڈ پروسیسنگ
	body := getText(v.Message)
	dataMutex.RLock()
	prefix := data.Prefix
	dataMutex.RUnlock()

	cmd := strings.ToLower(body)
	args := []string{}
	
	if strings.HasPrefix(cmd, prefix) {
		split := strings.Fields(cmd[len(prefix):])
		if len(split) > 0 {
			cmd = split[0]
			args = split[1:]
		}
	} else {
		split := strings.Fields(cmd)
		if len(split) > 0 {
			cmd = split[0]
			args = split[1:]
		}
	}

	if !canExecute(client, v, cmd) { return }

	fullArgs := strings.Join(args, " ")
	fmt.Printf("📩 CMD: %s | Chat: %s\n", cmd, v.Info.Chat.User)

	// پلگ ان سسٹم - ہر کمانڈ الگ فائل سے
	switch cmd {
	// مینیو سسٹم
	case "menu", "help", "list":
		HandleMenu(client, v)
	case "ping":
		HandlePing(client, v)
	case "id":
		HandleID(client, v)
	case "owner":
		HandleOwner(client, v)

	// سیٹنگز
	case "alwaysonline": HandleAlwaysOnline(client, v)
	case "autoread": HandleAutoRead(client, v)
	case "autoreact": HandleAutoReact(client, v)
	case "autostatus": HandleAutoStatus(client, v)
	case "statusreact": HandleStatusReact(client, v)
	case "addstatus": HandleAddStatus(client, v, args)
	case "delstatus": HandleDelStatus(client, v, args)
	case "liststatus": HandleListStatus(client, v)
	case "setprefix": HandleSetPrefix(client, v, args)
	case "mode": HandleMode(client, v, args)
	case "readallstatus": HandleReadAllStatus(client, v)

	// سیکورٹی
	case "antilink": HandleAntilink(client, v)
	case "antipic": HandleAntipic(client, v)
	case "antivideo": HandleAntivideo(client, v)
	case "antisticker": HandleAntisticker(client, v)

	// گروپ
	case "kick": HandleKick(client, v, args)
	case "add": HandleAdd(client, v, args)
	case "promote": HandlePromote(client, v, args)
	case "demote": HandleDemote(client, v, args)
	case "tagall": HandleTagAll(client, v, args)
	case "hidetag": HandleHideTag(client, v, args)
	case "group": HandleGroup(client, v, args)
	case "del", "delete": HandleDelete(client, v)

	// ڈاؤن لوڈرز
	case "tiktok", "tt": HandleTikTok(client, v, fullArgs)
	case "fb", "facebook": HandleFacebook(client, v, fullArgs)
	case "insta", "ig": HandleInstagram(client, v, fullArgs)
	case "pin", "pinterest": HandlePinterest(client, v, fullArgs)
	case "ytmp3": HandleYouTubeMP3(client, v, fullArgs)
	case "ytmp4": HandleYouTubeMP4(client, v, fullArgs)

	// ٹولز
	case "sticker", "s": HandleSticker(client, v)
	case "toimg": HandleToImg(client, v)
	case "tovideo": HandleToVideo(client, v)
	case "removebg": HandleRemoveBG(client, v)
	case "remini": HandleRemini(client, v)
	case "tourl": HandleToURL(client, v)
	case "weather": HandleWeather(client, v, fullArgs)
	case "translate", "tr": HandleTranslate(client, v, args)
	case "vv": HandleVV(client, v)

	// ڈیٹا
	case "data":
		reply(client, v.Info.Chat, "📂 Data is safe in MongoDB.")
	}
}

// --- بنیادی ہیلپر فنکشنز ---
func react(client *whatsmeow.Client, chat types.JID, msgID types.MessageID, emoji string) {
	client.SendMessage(context.Background(), chat, &waProto.Message{
		ReactionMessage: &waProto.ReactionMessage{
			Key: &waProto.MessageKey{
				RemoteJID: proto.String(chat.String()),
				ID:        proto.String(string(msgID)),
				FromMe:    proto.Bool(true),
			},
			Text:              proto.String(emoji),
			SenderTimestampMS: proto.Int64(time.Now().UnixMilli()),
		},
	})
}

func reply(client *whatsmeow.Client, chat types.JID, text string) {
	client.SendMessage(context.Background(), chat, &waProto.Message{
		ExtendedTextMessage: &waProto.ExtendedTextMessage{
			Text: proto.String(text),
		},
	})
}

func getText(m *waProto.Message) string {
	if m.Conversation != nil {
		return *m.Conversation
	}
	if m.ExtendedTextMessage != nil {
		return *m.ExtendedTextMessage.Text
	}
	if m.ImageMessage != nil && m.ImageMessage.Caption != nil {
		return *m.ImageMessage.Caption
	}
	if m.VideoMessage != nil && m.VideoMessage.Caption != nil {
		return *m.VideoMessage.Caption
	}
	return ""
}

func isOwner(client *whatsmeow.Client, sender types.JID) bool {
	if client.Store.ID == nil {
		return false
	}

	botNum := cleanNumber(client.Store.ID.User)
	senderNum := cleanNumber(sender.User)
	
	return botNum == senderNum
}

func cleanNumber(num string) string {
	num = strings.ReplaceAll(num, "+", "")
	if strings.Contains(num, ":") {
		num = strings.Split(num, ":")[0]
	}
	if strings.Contains(num, "@") {
		num = strings.Split(num, "@")[0]
	}
	return num
}

func canExecute(client *whatsmeow.Client, v *events.Message, cmd string) bool {
	if isOwner(client, v.Info.Sender) {
		return true
	}
	
	if !v.Info.IsGroup {
		return true
	}
	
	s := getGroupSettings(v.Info.Chat.String())
	if s.Mode == "private" {
		return false
	}
	if s.Mode == "admin" {
		return isAdmin(client, v.Info.Chat, v.Info.Sender)
	}
	return true
}

func isAdmin(client *whatsmeow.Client, chat, user types.JID) bool {
	info, err := client.GetGroupInfo(context.Background(), chat)
	if err != nil {
		return false
	}
	
	for _, p := range info.Participants {
		if p.JID.User == user.User && (p.IsAdmin || p.IsSuperAdmin) {
			return true
		}
	}
	return false
}

// --- ڈیٹا بیس فنکشنز ---
func getGroupSettings(id string) *GroupSettings {
	cacheMutex.RLock()
	if s, ok := groupCache[id]; ok {
		cacheMutex.RUnlock()
		return s
	}
	cacheMutex.RUnlock()
	
	s := &GroupSettings{
		ChatID:         id,
		Mode:           "public",
		Antilink:       false,
		AntilinkAdmin:  true,
		AntilinkAction: "delete",
		AntiPic:        false,
		AntiVideo:      false,
		AntiSticker:    false,
		Warnings:       make(map[string]int),
	}
	
	cacheMutex.Lock()
	groupCache[id] = s
	cacheMutex.Unlock()
	
	return s
}

func saveGroupSettings(s *GroupSettings) {
	cacheMutex.Lock()
	groupCache[s.ChatID] = s
	cacheMutex.Unlock()
}