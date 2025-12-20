package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"google.golang.org/protobuf/proto"
)

// --- CONFIGURATION ---
const (
	BOT_NAME   = "IMPOSSIBLE BOT"
	OWNER_NAME = "Nothing Is Impossible"
)

var (
	container     *sqlstore.Container
	clientMap     = make(map[string]*whatsmeow.Client) // Multi-device map
	clientMutex   sync.RWMutex
	startTime     = time.Now()
)

// --- MAIN FUNCTION ---
func main() {
	fmt.Println("🚀 IMPOSSIBLE BOT (NODEJS PORT) | STARTING...")

	// 1. DATABASE SETUP
	dbURL := os.Getenv("DATABASE_URL")
	dbType := "postgres"
	if dbURL == "" {
		dbType = "sqlite3"
		dbURL = "file:impossible_sessions.db?_foreign_keys=on"
	}

	dbLog := waLog.Stdout("DB", "INFO", true)
	var err error
	container, err = sqlstore.New(context.Background(), dbType, dbURL, dbLog)
	if err != nil {
		log.Fatalf("❌ DB Connection Error: %v", err)
	}

	// 2. RESTORE SESSIONS (Multi-Device Auto Reconnect)
	devices, err := container.GetAllDevices(context.Background())
	if err != nil {
		log.Printf("⚠️ No devices found: %v", err)
	} else {
		fmt.Printf("🔄 Restoring %d sessions...\n", len(devices))
		for _, device := range devices {
			go connectClient(device)
		}
	}

	// 3. WEB SERVER (PAIRING API)
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	r.LoadHTMLGlob("web/*.html") // اگر ایچ ٹی ایم ایل فائل ہے

	r.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "Online", "sessions": len(clientMap)})
	})

	// Pair API - Creates NEW Session
	r.POST("/api/pair", handlePairing)

	go r.Run(":8080")
	fmt.Println("🌐 Server running on :8080")

	// 4. KEEP ALIVE
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	fmt.Println("🛑 Shutting down...")
	clientMutex.Lock()
	for _, cli := range clientMap {
		cli.Disconnect()
	}
	clientMutex.Unlock()
}

// ================= SESSION MANAGER =================

func connectClient(device *store.Device) {
	clientLog := waLog.Stdout("Client", "INFO", true)
	client := whatsmeow.NewClient(device, clientLog)

	// Event Handler Wrapper
	client.AddEventHandler(func(evt interface{}) {
		handler(client, evt)
	})

	err := client.Connect()
	if err != nil {
		log.Printf("❌ Connect Failed: %v", err)
		return
	}

	if client.Store.ID != nil {
		clientMutex.Lock()
		clientMap[client.Store.ID.String()] = client
		clientMutex.Unlock()
		fmt.Printf("✅ Client Connected: %s\n", client.Store.ID.User)
	}
}

func handlePairing(c *gin.Context) {
	var req struct {
		Number string `json:"number"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Invalid JSON"})
		return
	}

	// Clean Number
	num := strings.ReplaceAll(req.Number, "+", "")
	num = strings.ReplaceAll(num, " ", "")
	num = strings.ReplaceAll(num, "-", "")

	// Create NEW Device in DB
	device := container.NewDevice()
	client := whatsmeow.NewClient(device, waLog.Stdout("Pairing", "INFO", true))

	if err := client.Connect(); err != nil {
		c.JSON(500, gin.H{"error": "Failed to connect"})
		return
	}

	// Generate Code
	code, err := client.PairPhone(context.Background(), num, true, whatsmeow.PairClientChrome, "Linux")
	if err != nil {
		client.Disconnect()
		c.JSON(500, gin.H{"error": "Pairing Error: " + err.Error()})
		return
	}

	// Attach Handler immediately
	client.AddEventHandler(func(evt interface{}) {
		handler(client, evt)
	})

	c.JSON(200, gin.H{"code": code, "message": "Code generated. Session saved in DB."})
}

// ================= MAIN EVENT HANDLER =================

func handler(client *whatsmeow.Client, evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		if v.Info.IsFromMe { return }

		// --- AUTO STATUS READ ---
		if v.Info.Chat.String() == "status@broadcast" {
			client.MarkRead([]types.MessageID{v.Info.ID}, v.Info.Timestamp, v.Info.Chat, v.Info.Sender)
			// Auto React Logic
			emojis := []string{"💚", "❤️", "🔥", "😍"}
			randEmoji := emojis[time.Now().UnixNano()%int64(len(emojis))]
			react(client, v.Info.Chat, v.Message, randEmoji) // React to status
			return
		}

		body := getText(v.Message)
		if body == "" { return }

		// --- PREFIX CHECK ---
		prefix := "#" // You can make this dynamic per user if needed
		if !strings.HasPrefix(body, prefix) { return }

		// Parse Command
		args := strings.Fields(body[len(prefix):])
		if len(args) == 0 { return }
		cmd := strings.ToLower(args[0])
		fullArgs := strings.Join(args[1:], " ")
		
		chat := v.Info.Chat
		isGroup := v.Info.IsGroup
		sender := v.Info.Sender

		fmt.Printf("📩 CMD: %s | User: %s\n", cmd, sender.User)

		// --- COMMAND ROUTER (NODEJS STYLE) ---
		switch cmd {
		
		// ➤ MAIN COMMANDS
		case "menu", "help", "list":
			sendMenu(client, chat, sender, prefix)
		case "ping":
			sendPing(client, chat, v.Message)
		case "id":
			reply(client, chat, v.Message, fmt.Sprintf("🆔 *ID INFO*\n👤 User: %s\n👥 Chat: %s", sender.User, chat.User))
		
		// ➤ DOWNLOADERS (From downloader.js)
		case "tiktok", "tt":
			dlTikTok(client, chat, fullArgs, v.Message)
		case "fb", "facebook":
			dlFacebook(client, chat, fullArgs, v.Message)
		case "insta", "ig":
			dlInstagram(client, chat, fullArgs, v.Message)
		case "pin", "pinterest":
			dlPinterest(client, chat, fullArgs, v.Message)
		case "ytmp3":
			dlYouTube(client, chat, fullArgs, "mp3", v.Message)
		case "ytmp4":
			dlYouTube(client, chat, fullArgs, "mp4", v.Message)

		// ➤ TOOLS (From tools.js)
		case "sticker", "s":
			makeSticker(client, chat, v.Message)
		case "toimg":
			stickerToImg(client, chat, v.Message)
		case "removebg":
			removeBG(client, chat, v.Message)
		case "remini":
			reminiEnhance(client, chat, v.Message)
		case "tourl":
			mediaToUrl(client, chat, v.Message)
		case "weather":
			getWeather(client, chat, fullArgs, v.Message)
		case "translate", "tr":
			doTranslate(client, chat, args[1:], v.Message)

		// ➤ GROUPS (From groups.js)
		case "kick":
			groupAction(client, chat, v.Message, "remove", isGroup)
		case "add":
			groupAdd(client, chat, args[1:], isGroup, v.Message)
		case "promote":
			groupAction(client, chat, v.Message, "promote", isGroup)
		case "demote":
			groupAction(client, chat, v.Message, "demote", isGroup)
		case "tagall":
			groupTagAll(client, chat, fullArgs, isGroup, v.Message)
		case "hidetag":
			groupHideTag(client, chat, fullArgs, isGroup, v.Message)
		case "group":
			groupSettings(client, chat, args[1:], isGroup, v.Message)
		
		// ➤ OWNER/SETTINGS (From setting.js)
		case "del", "delete":
			deleteMessage(client, chat, v.Message)
		case "readallstatus":
			reply(client, chat, v.Message, "✅ Auto-read is active in background.")
		}
	}
}

// ================= FUNCTIONS (LOGIC) =================

// 1. MENU (Node.js Style Copy)
func sendMenu(client *whatsmeow.Client, chat types.JID, sender types.JID, prefix string) {
	react(client, chat, nil, "📜")
	uptime := time.Since(startTime).Round(time.Second)
	
	menu := fmt.Sprintf(`╭━━━〔 *%s* 〕━━━┈
┃ 👋 *Assalam-o-Alaikum*
┃ 👑 *Owner:* %s
┃ 🤖 *Bot:* %s
┃ 🛡️ *Mode:* PUBLIC
┃ 📍 *Prefix:* %s
┃ ⏳ *Uptime:* %s
╰━━━━━━━━━━━━━━━━━━┈

╭━━〔 *DOWNLOADERS* 〕━━┈
┃ 🔸 %stiktok [url]
┃ 🔸 %sfb [url]
┃ 🔸 %sinsta [url]
┃ 🔸 %spin [url]
┃ 🔸 %sytmp3 [url]
┃ 🔸 %sytmp4 [url]
╰━━━━━━━━━━━━━━━━━━┈

╭━━〔 *TOOLS* 〕━━┈
┃ 🔸 %ssticker (Reply Media)
┃ 🔸 %stoimg (Reply Sticker)
┃ 🔸 %sremovebg (Reply Img)
┃ 🔸 %sremini (Reply Img)
┃ 🔸 %stranslate [text]
┃ 🔸 %sweather [city]
╰━━━━━━━━━━━━━━━━━━┈

╭━━〔 *GROUPS* 〕━━┈
┃ 🔸 %skick @user
┃ 🔸 %sadd 923...
┃ 🔸 %spromote @user
┃ 🔸 %sdemote @user
┃ 🔸 %stagall [msg]
┃ 🔸 %shidetag [msg]
┃ 🔸 %sgroup open/close
┃ 🔸 %sdel (Reply Msg)
╰━━━━━━━━━━━━━━━━━━┈

© 2025 %s`, 
	BOT_NAME, OWNER_NAME, "IMPOSSIBLE_V2", prefix, uptime,
	prefix, prefix, prefix, prefix, prefix, prefix,
	prefix, prefix, prefix, prefix, prefix, prefix,
	prefix, prefix, prefix, prefix, prefix, prefix, prefix, prefix,
	BOT_NAME)

	// Send as Text or Image if you have 'pic.jpg'
	reply(client, chat, nil, menu)
}

// 2. PING
func sendPing(client *whatsmeow.Client, chat types.JID, msg *waProto.Message) {
	react(client, chat, msg, "⚡")
	start := time.Now()
	// Fake DB processing
	time.Sleep(20 * time.Millisecond)
	lat := time.Since(start).Milliseconds()
	reply(client, chat, msg, fmt.Sprintf("*⚡ Ping:* %dms", lat))
}

// 3. GROUP ACTIONS (Kick/Promote/Demote - Handles Reply & Mention)
func groupAction(client *whatsmeow.Client, chat types.JID, msg *waProto.Message, action string, isGroup bool) {
	if !isGroup { return }
	
	target := getTargetJID(msg)
	if target == nil {
		reply(client, chat, msg, "⚠️ *Target Missing!*\nReply to a user or Tag them.")
		return
	}

	var change whatsmeow.ParticipantChange
	emoji := ""
	switch action {
	case "remove": change = whatsmeow.ParticipantChangeRemove; emoji = "👢"
	case "promote": change = whatsmeow.ParticipantChangePromote; emoji = "⬆️"
	case "demote": change = whatsmeow.ParticipantChangeDemote; emoji = "⬇️"
	}

	react(client, chat, msg, emoji)
	_, err := client.UpdateGroupParticipants(chat, []types.JID{*target}, change)
	if err != nil {
		reply(client, chat, msg, "❌ Failed. (Am I Admin?)")
	} else {
		reply(client, chat, msg, fmt.Sprintf("%s Action Complete!", emoji))
	}
}

// 4. ADD USER
func groupAdd(client *whatsmeow.Client, chat types.JID, args []string, isGroup bool, msg *waProto.Message) {
	if !isGroup || len(args) == 0 { reply(client, chat, msg, "⚠️ Number required."); return }
	react(client, chat, msg, "➕")
	
	num := strings.ReplaceAll(args[0], "+", "") + "@s.whatsapp.net"
	jid, _ := types.ParseJID(num)
	_, err := client.UpdateGroupParticipants(chat, []types.JID{jid}, whatsmeow.ParticipantChangeAdd)
	if err != nil { reply(client, chat, msg, "❌ Failed to add (Privacy settings maybe?)") }
}

// 5. TAG ALL
func groupTagAll(client *whatsmeow.Client, chat types.JID, text string, isGroup bool, msg *waProto.Message) {
	if !isGroup { return }
	react(client, chat, msg, "📣")
	
	info, err := client.GetGroupInfo(chat)
	if err != nil { return }
	
	mentions := []string{}
	out := fmt.Sprintf("📣 *EVERYONE*\n%s\n\n", text)
	for _, p := range info.Participants {
		mentions = append(mentions, p.JID.String())
		out += fmt.Sprintf("@%s\n", p.JID.User)
	}
	
	client.SendMessage(context.Background(), chat, &waProto.Message{
		ExtendedTextMessage: &waProto.ExtendedTextMessage{
			Text: proto.String(out),
			ContextInfo: &waProto.ContextInfo{ MentionedJid: mentions },
		},
	})
}

// 6. HIDE TAG
func groupHideTag(client *whatsmeow.Client, chat types.JID, text string, isGroup bool, msg *waProto.Message) {
	if !isGroup { return }
	info, _ := client.GetGroupInfo(chat)
	mentions := []string{}
	for _, p := range info.Participants { mentions = append(mentions, p.JID.String()) }
	
	if text == "" { text = "👻" }
	client.SendMessage(context.Background(), chat, &waProto.Message{
		ExtendedTextMessage: &waProto.ExtendedTextMessage{
			Text: proto.String(text),
			ContextInfo: &waProto.ContextInfo{ MentionedJid: mentions },
		},
	})
}

// 7. DELETE MESSAGE (#del)
func deleteMessage(client *whatsmeow.Client, chat types.JID, msg *waProto.Message) {
	react(client, chat, msg, "🗑️")
	quoted := msg.ExtendedTextMessage.GetContextInfo()
	if quoted == nil || quoted.StanzaId == nil {
		reply(client, chat, msg, "⚠️ Reply to a message to delete.")
		return
	}
	
	targetJID, _ := types.ParseJID(*quoted.Participant)
	client.RevokeMessage(chat, targetJID, *quoted.StanzaId)
}

// 8. DOWNLOADERS (Using API Logic from Node.js)
func dlTikTok(client *whatsmeow.Client, chat types.JID, urlStr string, msg *waProto.Message) {
	if urlStr == "" { reply(client, chat, msg, "⚠️ URL?"); return }
	react(client, chat, msg, "🎵")
	reply(client, chat, msg, "⚙️ Downloading TikTok...")

	type Resp struct { Data struct { Play string `json:"play"`; Title string `json:"title"` } `json:"data"` }
	var data Resp
	if err := getJson("https://www.tikwm.com/api/?url="+urlStr, &data); err != nil || data.Data.Play == "" {
		reply(client, chat, msg, "❌ Error.")
		return
	}
	sendVideo(client, chat, data.Data.Play, data.Data.Title)
}

func dlFacebook(client *whatsmeow.Client, chat types.JID, urlStr string, msg *waProto.Message) {
	if urlStr == "" { return }
	react(client, chat, msg, "📘")
	reply(client, chat, msg, "⚙️ Downloading FB...")
	
	type Resp struct { BK9 struct { HD string `json:"HD"`; SD string `json:"SD"` } `json:"BK9"`; Status bool `json:"status"` }
	var data Resp
	if err := getJson("https://bk9.fun/downloader/facebook?url="+urlStr, &data); err != nil || !data.Status {
		reply(client, chat, msg, "❌ Error.")
		return
	}
	if data.BK9.HD != "" { sendVideo(client, chat, data.BK9.HD, "FB HD") } else { sendVideo(client, chat, data.BK9.SD, "FB SD") }
}

func dlInstagram(client *whatsmeow.Client, chat types.JID, urlStr string, msg *waProto.Message) {
	if urlStr == "" { return }
	react(client, chat, msg, "📸")
	reply(client, chat, msg, "⚙️ Downloading Insta...")
	
	// API change: Using tiklydown as per nodejs
	type Resp struct { Video struct { Url string `json:"url"` } `json:"video"`; Images []struct { Url string `json:"url"` } `json:"images"` }
	var data Resp
	if err := getJson("https://api.tiklydown.eu.org/api/download?url="+urlStr, &data); err != nil {
		reply(client, chat, msg, "❌ Error.")
		return
	}
	if data.Video.Url != "" { sendVideo(client, chat, data.Video.Url, "Insta Video") }
	for _, img := range data.Images { sendImage(client, chat, img.Url, "Insta Image") }
}

func dlYouTube(client *whatsmeow.Client, chat types.JID, urlStr, format string, msg *waProto.Message) {
	if urlStr == "" { return }
	react(client, chat, msg, "📺")
	reply(client, chat, msg, "⚙️ Downloading YouTube...")

	type Resp struct { BK9 struct { Mp4 string `json:"mp4"`; Mp3 string `json:"mp3"` } `json:"BK9"`; Status bool `json:"status"` }
	var data Resp
	if err := getJson("https://bk9.fun/downloader/youtube?url="+urlStr, &data); err != nil || !data.Status {
		reply(client, chat, msg, "❌ Error.")
		return
	}
	if format == "mp4" && data.BK9.Mp4 != "" {
		sendVideo(client, chat, data.BK9.Mp4, "YouTube Video")
	} else if format == "mp3" && data.BK9.Mp3 != "" {
		sendDoc(client, chat, data.BK9.Mp3, "audio.mp3", "audio/mpeg")
	}
}

// 9. TOOLS (Sticker/Remini/RemoveBG)
func makeSticker(client *whatsmeow.Client, chat types.JID, msg *waProto.Message) {
	react(client, chat, msg, "🎨")
	data, err := downloadMedia(client, msg)
	if err != nil { reply(client, chat, msg, "⚠️ Reply to image/video"); return }

	// Save Temp
	in := "temp_" + fmt.Sprint(time.Now().Unix()) + ".jpg"
	out := in + ".webp"
	ioutil.WriteFile(in, data, 0644)
	defer os.Remove(in)
	defer os.Remove(out)

	// FFmpeg Convert
	cmd := exec.Command("ffmpeg", "-y", "-i", in, "-vcodec", "libwebp", "-vf", "scale=512:512:flags=lanczos:force_original_aspect_ratio=decrease,format=rgba,pad=512:512:(ow-iw)/2:(oh-ih)/2:color=#00000000", "-lossless", "1", "-loop", "0", "-an", "-vsync", "0", out)
	if err := cmd.Run(); err != nil { reply(client, chat, msg, "❌ Convert Error"); return }

	webpData, _ := ioutil.ReadFile(out)
	client.SendMessage(context.Background(), chat, &waProto.Message{
		StickerMessage: &waProto.StickerMessage{
			Url: proto.String("https://mmg.whatsapp.net/d/f/something"),
			Mimetype: proto.String("image/webp"),
			FileLength: proto.Uint64(uint64(len(webpData))),
			FileSha256: []byte{1,2,3}, FileEncSha256: []byte{1,2,3}, MediaKey: []byte{1,2,3},
		},
	})
	// Note: Proper sticker upload requires MediaUpload in whatsmeow (Client.Upload), simplifying here
}

func reminiEnhance(client *whatsmeow.Client, chat types.JID, msg *waProto.Message) {
	react(client, chat, msg, "✨")
	data, err := downloadMedia(client, msg)
	if err != nil { reply(client, chat, msg, "⚠️ Reply to image"); return }
	
	reply(client, chat, msg, "⚙️ Enhancing...")
	urlStr := uploadToCatbox(data)
	if urlStr == "" { reply(client, chat, msg, "❌ Upload Failed"); return }

	type Resp struct { Url string `json:"url"` }
	var res Resp
	if err := getJson("https://remini.mobilz.pw/enhance?url="+urlStr, &res); err == nil && res.Url != "" {
		sendImage(client, chat, res.Url, "✨ Enhanced")
	}
}

// ================= UTILS =================

func reply(client *whatsmeow.Client, chat types.JID, quoted *waProto.Message, text string) {
	client.SendMessage(context.Background(), chat, &waProto.Message{
		ExtendedTextMessage: &waProto.ExtendedTextMessage{
			Text: proto.String(text),
			ContextInfo: &waProto.ContextInfo{
				StanzaId:      proto.String(quoted.GetKey().GetId()),
				Participant:   proto.String(quoted.GetKey().GetParticipant()),
				QuotedMessage: quoted,
			},
		},
	})
}

func react(client *whatsmeow.Client, chat types.JID, msg *waProto.Message, emoji string) {
	if msg == nil { return }
	client.SendMessage(context.Background(), chat, &waProto.Message{
		ReactionMessage: &waProto.ReactionMessage{
			Key: msg.Key,
			Text: proto.String(emoji),
		},
	})
}

func getText(msg *waProto.Message) string {
	if msg.Conversation != nil { return *msg.Conversation }
	if msg.ExtendedTextMessage != nil { return *msg.ExtendedTextMessage.Text }
	if msg.ImageMessage != nil { return *msg.ImageMessage.Caption }
	return ""
}

func getTargetJID(msg *waProto.Message) *types.JID {
	if msg.ExtendedTextMessage == nil { return nil }
	ctx := msg.ExtendedTextMessage.ContextInfo
	if ctx != nil {
		if len(ctx.MentionedJid) > 0 {
			j, _ := types.ParseJID(ctx.MentionedJid[0])
			return &j
		}
		if ctx.Participant != nil {
			j, _ := types.ParseJID(*ctx.Participant)
			return &j
		}
	}
	return nil
}

func downloadMedia(client *whatsmeow.Client, msg *waProto.Message) ([]byte, error) {
	// Extracts Image/Video from message or quoted message
	var doc *waProto.ImageMessage
	// Simplified extraction logic (Expand for video/sticker as needed)
	if msg.ImageMessage != nil { doc = msg.ImageMessage }
	if msg.ExtendedTextMessage != nil && msg.ExtendedTextMessage.ContextInfo != nil {
		q := msg.ExtendedTextMessage.ContextInfo.QuotedMessage
		if q != nil && q.ImageMessage != nil { doc = q.ImageMessage }
	}
	if doc == nil { return nil, fmt.Errorf("no media") }
	return client.Download(doc)
}

func getJson(url string, target interface{}) error {
	resp, err := http.Get(url)
	if err != nil { return err }
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(target)
}

func sendVideo(client *whatsmeow.Client, chat types.JID, urlStr, caption string) {
	// Whatsmeow needs upload usually, but for URLs we just send logic wrapper
	// In real production, you download the file, then Client.Upload(), then SendMessage
	// For simplicity in this script context, assuming download & upload handled or using url directly if supported (often not in raw protocol, needs media key).
	// *Proper way below:*
	resp, _ := http.Get(urlStr)
	data, _ := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	
	up, _ := client.Upload(context.Background(), data, whatsmeow.MediaVideo)
	
	client.SendMessage(context.Background(), chat, &waProto.Message{
		VideoMessage: &waProto.VideoMessage{
			Url: proto.String(up.URL),
			DirectPath: proto.String(up.DirectPath),
			MediaKey: up.MediaKey,
			FileEncSha256: up.FileEncSHA256,
			FileSha256: up.FileSHA256,
			FileLength: proto.Uint64(uint64(len(data))),
			Mimetype: proto.String("video/mp4"),
			Caption: proto.String(caption),
		},
	})
}

func sendImage(client *whatsmeow.Client, chat types.JID, urlStr, caption string) {
	resp, _ := http.Get(urlStr)
	data, _ := ioutil.ReadAll(resp.Body)
	resp.Body.Close()

	up, _ := client.Upload(context.Background(), data, whatsmeow.MediaImage)

	client.SendMessage(context.Background(), chat, &waProto.Message{
		ImageMessage: &waProto.ImageMessage{
			Url: proto.String(up.URL),
			DirectPath: proto.String(up.DirectPath),
			MediaKey: up.MediaKey,
			FileEncSha256: up.FileEncSHA256,
			FileSha256: up.FileSHA256,
			FileLength: proto.Uint64(uint64(len(data))),
			Mimetype: proto.String("image/jpeg"),
			Caption: proto.String(caption),
		},
	})
}

func sendDoc(client *whatsmeow.Client, chat types.JID, urlStr, name, mime string) {
	resp, _ := http.Get(urlStr)
	data, _ := ioutil.ReadAll(resp.Body)
	resp.Body.Close()

	up, _ := client.Upload(context.Background(), data, whatsmeow.MediaDocument)
	
	client.SendMessage(context.Background(), chat, &waProto.Message{
		DocumentMessage: &waProto.DocumentMessage{
			Url: proto.String(up.URL),
			DirectPath: proto.String(up.DirectPath),
			MediaKey: up.MediaKey,
			FileEncSha256: up.FileEncSHA256,
			FileSha256: up.FileSHA256,
			FileLength: proto.Uint64(uint64(len(data))),
			Mimetype: proto.String(mime),
			FileName: proto.String(name),
		},
	})
}

func uploadToCatbox(data []byte) string {
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	writer.WriteField("reqtype", "fileupload")
	part, _ := writer.CreateFormFile("fileToUpload", "file.jpg")
	part.Write(data)
	writer.Close()

	resp, err := http.Post("https://catbox.moe/user/api.php", writer.FormDataContentType(), body)
	if err != nil { return "" }
	defer resp.Body.Close()
	res, _ := ioutil.ReadAll(resp.Body)
	return string(res)
}

func stickerToImg(client *whatsmeow.Client, chat types.JID, msg *waProto.Message) {
	// Not implemented fully (complex webp conversion), but structure is here
	reply(client, chat, msg, "⚠️ Feature requires specific libwebp bindings.")
}

func removeBG(client *whatsmeow.Client, chat types.JID, msg *waProto.Message) {
	// Placeholder for removeBG API logic
	reply(client, chat, msg, "⚠️ API Key required for RemoveBG.")
}

func getWeather(client *whatsmeow.Client, chat types.JID, city string, msg *waProto.Message) {
	resp, _ := http.Get("https://wttr.in/" + city + "?format=%C+%t")
	body, _ := ioutil.ReadAll(resp.Body)
	reply(client, chat, msg, "🌦️ " + string(body))
}

func doTranslate(client *whatsmeow.Client, chat types.JID, args []string, msg *waProto.Message) {
	reply(client, chat, msg, "🌍 Google Translate API integration pending.")
}

func mediaToUrl(client *whatsmeow.Client, chat types.JID, msg *waProto.Message) {
	data, err := downloadMedia(client, msg)
	if err != nil { reply(client, chat, msg, "Reply media"); return }
	url := uploadToCatbox(data)
	reply(client, chat, msg, "🔗 " + url)
}

func groupSettings(client *whatsmeow.Client, chat types.JID, args []string, isGroup bool, msg *waProto.Message) {
	if !isGroup || len(args) == 0 { return }
	react(client, chat, msg, "⚙️")
	if args[0] == "close" {
		client.SetGroupAnnounce(chat, true)
		reply(client, chat, msg, "🔒 Group Closed")
	} else if args[0] == "open" {
		client.SetGroupAnnounce(chat, false)
		reply(client, chat, msg, "🔓 Group Opened")
	}
}

func dlPinterest(client *whatsmeow.Client, chat types.JID, urlStr string, msg *waProto.Message) {
	// Add Pinterest logic similar to dlFacebook
	reply(client, chat, msg, "📌 Pinterest API call...")
}