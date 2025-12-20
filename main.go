package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
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

// --- ⚙️ CONFIGURATION ---
const (
	BOT_NAME     = "IMPOSSIBLE BOT V3"
	OWNER_NAME   = "Nothing Is Impossible"
	OWNER_NUMBER = "92311xxxxxxx" // اپنا نمبر یہاں لکھیں
)

// --- 💾 DATA STRUCTURES ---
type GroupSettings struct {
	Mode           string         `json:"mode"`            // public, private, admin
	Antilink       bool           `json:"antilink"`        // true/false
	AntilinkAdmin  bool           `json:"antilink_admin"`  // true = admin allow
	AntilinkAction string         `json:"antilink_action"` // delete, kick, warn
	Warnings       map[string]int `json:"warnings"`        // UserID -> Count
}

type BotData struct {
	Prefix   string                    `json:"prefix"`
	Settings map[string]*GroupSettings `json:"groups"` // ChatID -> Settings
}

// --- 🔄 SETUP STATE (For Interactive Commands) ---
type SetupState struct {
	Stage   int    // 1: Ask Admin, 2: Ask Action
	GroupID string
	User    string
}

var (
	container   *sqlstore.Container
	clientMap   = make(map[string]*whatsmeow.Client)
	clientMutex sync.RWMutex
	startTime   = time.Now()

	// Data Persistence
	data      BotData
	dataMutex sync.RWMutex
	dataFile  = "bot_data.json"

	// Interactive Setup Map
	setupMap = make(map[string]*SetupState) // UserID -> State
)

// --- 🚀 MAIN START ---
func main() {
	fmt.Println("🚀 IMPOSSIBLE BOT FINAL ULTIMATE | STARTING...")

	// 1. Load Settings
	loadData()

	// 2. DB Connection
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
		log.Fatalf("❌ DB Error: %v", err)
	}

	// 3. Restore Sessions
	devices, err := container.GetAllDevices(context.Background())
	if err == nil {
		fmt.Printf("🔄 Restoring %d sessions...\n", len(devices))
		for _, device := range devices {
			go connectClient(device)
		}
	}

	// 4. Web Server
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	r.LoadHTMLGlob("web/*.html")

	r.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "Online", "sessions": len(clientMap)})
	})
	r.POST("/api/pair", handlePairing)

	go r.Run(":8080")
	fmt.Println("🌐 Server running on :8080")

	// 5. Keep Alive
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	saveData()
	clientMutex.Lock()
	for _, cli := range clientMap {
		cli.Disconnect()
	}
	clientMutex.Unlock()
}

// --- 🔌 CLIENT & PAIRING ---
func connectClient(device *store.Device) {
	client := whatsmeow.NewClient(device, waLog.Stdout("Client", "INFO", true))
	client.AddEventHandler(func(evt interface{}) {
		handler(client, evt)
	})

	if err := client.Connect(); err == nil && client.Store.ID != nil {
		clientMutex.Lock()
		clientMap[client.Store.ID.String()] = client
		clientMutex.Unlock()
		fmt.Printf("✅ Connected: %s\n", client.Store.ID.User)
	}
}

func handlePairing(c *gin.Context) {
	var req struct{ Number string `json:"number"` }
	if c.BindJSON(&req) != nil { return }
	num := strings.ReplaceAll(req.Number, " ", "")
	num = strings.ReplaceAll(num, "+", "")

	device := container.NewDevice()
	client := whatsmeow.NewClient(device, waLog.Stdout("Pairing", "INFO", true))

	if err := client.Connect(); err != nil {
		c.JSON(500, gin.H{"error": "Connection Failed"})
		return
	}

	code, err := client.PairPhone(context.Background(), num, true, whatsmeow.PairClientChrome, "Linux")
	if err != nil {
		client.Disconnect()
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	client.AddEventHandler(func(evt interface{}) {
		handler(client, evt)
	})
	c.JSON(200, gin.H{"code": code})
}

// --- 📡 EVENT HANDLER ---
func handler(client *whatsmeow.Client, evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		if v.Info.IsFromMe { return }

		chatID := v.Info.Chat.String()
		senderID := v.Info.Sender.String()
		isGroup := v.Info.IsGroup

		// 1. INTERACTIVE SETUP FLOW (Antilink)
		if state, ok := setupMap[senderID]; ok && state.GroupID == chatID {
			handleSetupResponse(client, v, state)
			return
		}

		// 2. AUTO READ STATUS
		if chatID == "status@broadcast" {
			client.MarkRead(context.Background(), []types.MessageID{v.Info.ID}, v.Info.Timestamp, v.Info.Chat, v.Info.Sender, types.ReceiptTypeRead)
			return
		}

		// 3. ANTILINK CHECK
		if isGroup {
			checkAntilink(client, v)
		}

		// 4. COMMAND PARSING
		body := getText(v.Message)
		if !strings.HasPrefix(body, data.Prefix) { return }

		args := strings.Fields(body[len(data.Prefix):])
		if len(args) == 0 { return }
		cmd := strings.ToLower(args[0])
		fullArgs := strings.Join(args[1:], " ")

		// 5. MODE & PERMISSION CHECK
		if !canExecute(client, v, cmd) { return }

		fmt.Printf("📩 CMD: %s | Chat: %s\n", cmd, v.Info.Chat.User)

		// --- COMMAND ROUTER ---
		switch cmd {
		// ➤ MAIN
		case "menu", "help", "list": sendMenu(client, v.Info.Chat)
		case "ping": sendPing(client, v.Info.Chat, v.Message)
		case "id": sendID(client, v)
		case "owner": sendOwner(client, v.Info.Chat, v.Info.Sender)

		// ➤ SETTINGS
		case "setprefix":
			if fullArgs != "" {
				data.Prefix = args[1]
				saveData()
				reply(client, v.Info.Chat, v.Message, makeCard("SETTINGS", "✅ Prefix updated: "+data.Prefix))
			}
		case "mode": handleMode(client, v, args)
		case "antilink": startAntilinkSetup(client, v)

		// ➤ GROUPS
		case "kick": groupAction(client, v.Info.Chat, v.Message, "remove", isGroup)
		case "add": groupAdd(client, v.Info.Chat, args[1:], isGroup)
		case "promote": groupAction(client, v.Info.Chat, v.Message, "promote", isGroup)
		case "demote": groupAction(client, v.Info.Chat, v.Message, "demote", isGroup)
		case "tagall": groupTagAll(client, v.Info.Chat, fullArgs, isGroup)
		case "hidetag": groupHideTag(client, v.Info.Chat, fullArgs, isGroup)
		case "group": groupOpenClose(client, v.Info.Chat, args[1:], isGroup)
		case "del", "delete": deleteMsg(client, v.Info.Chat, v.Message)

		// ➤ DOWNLOADERS
		case "tiktok", "tt": dlTikTok(client, v.Info.Chat, fullArgs, v.Message)
		case "fb", "facebook": dlFacebook(client, v.Info.Chat, fullArgs, v.Message)
		case "insta", "ig": dlInstagram(client, v.Info.Chat, fullArgs, v.Message)
		case "pin", "pinterest": dlPinterest(client, v.Info.Chat, fullArgs, v.Message)
		case "ytmp3": dlYouTube(client, v.Info.Chat, fullArgs, "mp3", v.Message)
		case "ytmp4": dlYouTube(client, v.Info.Chat, fullArgs, "mp4", v.Message)

		// ➤ TOOLS
		case "sticker", "s": makeSticker(client, v.Info.Chat, v.Message)
		case "toimg": stickerToImg(client, v.Info.Chat, v.Message)
		case "removebg": removeBG(client, v.Info.Chat, v.Message)
		case "remini": reminiEnhance(client, v.Info.Chat, v.Message)
		case "tourl": mediaToUrl(client, v.Info.Chat, v.Message)
		case "weather": getWeather(client, v.Info.Chat, fullArgs, v.Message)
		case "tr", "translate": doTranslate(client, v.Info.Chat, args[1:], v.Message)
		}
	}
}

// --- 🛠️ LOGIC FUNCTIONS ---

// 1. MENU (User Style)
func sendMenu(client *whatsmeow.Client, chat types.JID) {
	react(client, chat, nil, "📜")
	menu := makeCard("IMPOSSIBLE BOT V3", fmt.Sprintf(`
👋 *Assalam-o-Alaikum*
👑 *Owner:* %s
🤖 *Mode:* Multi-Device
📍 *Prefix:* %s
⏳ *Uptime:* %s

╭━━〔 *SETTINGS* 〕━━┈
┃ 🔸 %santilink (Interactive)
┃ 🔸 %smode [public/private/admin]
┃ 🔸 %ssetprefix [sym]
┃ 🔸 %sid
┃ 🔸 %sowner
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
┃ 🔸 %stourl (Reply Media)
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
	OWNER_NAME, data.Prefix, time.Since(startTime).Round(time.Second),
	data.Prefix, data.Prefix, data.Prefix, data.Prefix, data.Prefix,
	data.Prefix, data.Prefix, data.Prefix, data.Prefix, data.Prefix, data.Prefix,
	data.Prefix, data.Prefix, data.Prefix, data.Prefix, data.Prefix, data.Prefix, data.Prefix,
	data.Prefix, data.Prefix, data.Prefix, data.Prefix, data.Prefix, data.Prefix, data.Prefix, data.Prefix,
	BOT_NAME))

	client.SendMessage(context.Background(), chat, &waProto.Message{Conversation: proto.String(menu)})
}

// 2. ID (Card Style)
func sendID(client *whatsmeow.Client, v *events.Message) {
	react(client, v.Info.Chat, v.Message, "🆔")
	
	u := v.Info.Sender.User
	c := v.Info.Chat.User
	t := "Private"
	if v.Info.IsGroup { t = "Group" }
	if v.Info.Chat.Server == "newsletter" { t = "Channel" }

	text := fmt.Sprintf("👤 *User:* %s\n👥 *Chat/Group:* %s\n🏷️ *Type:* %s", u, c, t)
	reply(client, v.Info.Chat, v.Message, makeCard("ID INFORMATION", text))
}

// 3. OWNER CHECK (LID/Number Matching)
func sendOwner(client *whatsmeow.Client, chat types.JID, sender types.JID) {
	botNum := client.Store.ID.User
	senderNum := sender.User
	
	status := "❌ You are NOT the Owner."
	if senderNum == botNum || senderNum == OWNER_NUMBER {
		status = "👑 You are the OWNER!"
	}

	reply(client, chat, nil, makeCard("OWNER VERIFICATION", fmt.Sprintf("🤖 Bot: %s\n👤 You: %s\n\n%s", botNum, senderNum, status)))
}

// 4. MODE HANDLER
func handleMode(client *whatsmeow.Client, v *events.Message, args []string) {
	if !isAdmin(client, v.Info.Chat, v.Info.Sender) && !isOwner(client, v.Info.Sender) {
		reply(client, v.Info.Chat, v.Message, "❌ Admin/Owner only.")
		return
	}
	if len(args) < 2 { reply(client, v.Info.Chat, v.Message, "⚠️ Use: public, private, admin"); return }
	
	m := strings.ToLower(args[1])
	if m!="public" && m!="private" && m!="admin" { return }
	
	s := getGroupSettings(v.Info.Chat.String())
	s.Mode = m
	saveData()
	reply(client, v.Info.Chat, v.Message, makeCard("MODE CHANGED", "🔒 New Mode: *"+strings.ToUpper(m)+"*"))
}

// 5. ANTILINK SETUP (Interactive)
func startAntilinkSetup(client *whatsmeow.Client, v *events.Message) {
	if !v.Info.IsGroup || !isAdmin(client, v.Info.Chat, v.Info.Sender) { return }
	
	setupMap[v.Info.Sender.String()] = &SetupState{Stage: 1, GroupID: v.Info.Chat.String(), User: v.Info.Sender.String()}
	reply(client, v.Info.Chat, v.Message, makeCard("ANTILINK SETUP (1/2)", "🛡️ *Allow Admin to send links?*\n\nType *Yes* or *No*"))
}

func handleSetupResponse(client *whatsmeow.Client, v *events.Message, state *SetupState) {
	txt := strings.ToLower(getText(v.Message))
	s := getGroupSettings(state.GroupID)

	if state.Stage == 1 {
		if txt == "yes" { s.AntilinkAdmin = true } else if txt == "no" { s.AntilinkAdmin = false } else { return }
		state.Stage = 2
		reply(client, v.Info.Chat, v.Message, makeCard("ANTILINK SETUP (2/2)", "⚡ *Choose Action:*\n\nType:\n*Delete*\n*Kick*\n*Warn*"))
		return
	}

	if state.Stage == 2 {
		if strings.Contains(txt, "kick") { s.AntilinkAction = "kick" }
		if strings.Contains(txt, "warn") { s.AntilinkAction = "warn" }
		if strings.Contains(txt, "delete") { s.AntilinkAction = "delete" }
		
		s.Antilink = true
		saveData()
		delete(setupMap, state.User)
		reply(client, v.Info.Chat, v.Message, makeCard("✅ ANTILINK ENABLED", fmt.Sprintf("👑 Admin Allowed: %v\n⚡ Action: %s", s.AntilinkAdmin, strings.ToUpper(s.AntilinkAction))))
	}
}

func checkAntilink(client *whatsmeow.Client, v *events.Message) {
	s := getGroupSettings(v.Info.Chat.String())
	if !s.Antilink { return }
	
	txt := getText(v.Message)
	if strings.Contains(txt, "chat.whatsapp.com") || strings.Contains(txt, "http") {
		if s.AntilinkAdmin && isAdmin(client, v.Info.Chat, v.Info.Sender) { return }
		
		// Delete
		client.RevokeMessage(context.Background(), v.Info.Chat, v.Info.Sender, v.Info.ID)
		
		if s.AntilinkAction == "kick" {
			client.UpdateGroupParticipants(context.Background(), v.Info.Chat, []types.JID{v.Info.Sender}, whatsmeow.ParticipantChangeRemove)
		} else if s.AntilinkAction == "warn" {
			s.Warnings[v.Info.Sender.String()]++
			saveData()
			if s.Warnings[v.Info.Sender.String()] >= 3 {
				client.UpdateGroupParticipants(context.Background(), v.Info.Chat, []types.JID{v.Info.Sender}, whatsmeow.ParticipantChangeRemove)
				client.SendMessage(context.Background(), v.Info.Chat, &waProto.Message{Conversation: proto.String("🚫 3 Warnings. Kicked.")})
				delete(s.Warnings, v.Info.Sender.String())
			} else {
				client.SendMessage(context.Background(), v.Info.Chat, &waProto.Message{Conversation: proto.String("⚠️ Warning! No Links.")})
			}
		}
	}
}

// --- 🌐 DOWNLOADERS ---
func dlTikTok(client *whatsmeow.Client, chat types.JID, url string, msg *waProto.Message) {
	react(client, chat, msg, "🎵")
	type R struct { Data struct { Play string `json:"play"`; Title string `json:"title"` } `json:"data"` }
	var res R
	if getJson("https://www.tikwm.com/api/?url="+url, &res) == nil && res.Data.Play != "" {
		sendVideo(client, chat, res.Data.Play, res.Data.Title)
	} else { reply(client, chat, msg, "❌ Error") }
}

func dlFacebook(client *whatsmeow.Client, chat types.JID, url string, msg *waProto.Message) {
	react(client, chat, msg, "📘")
	type R struct { BK9 struct { HD string `json:"HD"`; SD string `json:"SD"` } `json:"BK9"`; Status bool `json:"status"` }
	var res R
	if getJson("https://bk9.fun/downloader/facebook?url="+url, &res) == nil && res.Status {
		link := res.BK9.HD; if link == "" { link = res.BK9.SD }
		sendVideo(client, chat, link, "Facebook Video")
	} else { reply(client, chat, msg, "❌ Error") }
}

func dlInstagram(client *whatsmeow.Client, chat types.JID, url string, msg *waProto.Message) {
	react(client, chat, msg, "📸")
	type R struct { Video struct { Url string `json:"url"` } `json:"video"`; Images []struct { Url string `json:"url"` } `json:"images"` }
	var res R
	if getJson("https://api.tiklydown.eu.org/api/download?url="+url, &res) == nil {
		if res.Video.Url != "" { sendVideo(client, chat, res.Video.Url, "Insta Video") }
		for _, img := range res.Images { sendImage(client, chat, img.Url, "Insta Image") }
	} else { reply(client, chat, msg, "❌ Error") }
}

func dlPinterest(client *whatsmeow.Client, chat types.JID, url string, msg *waProto.Message) {
	react(client, chat, msg, "📌")
	type R struct { BK9 struct { Url string `json:"url"` } `json:"BK9"`; Status bool `json:"status"` }
	var res R
	if getJson("https://bk9.fun/downloader/pinterest?url="+url, &res) == nil && res.Status {
		if strings.Contains(res.BK9.Url, "mp4") { sendVideo(client, chat, res.BK9.Url, "Pin Video") } else { sendImage(client, chat, res.BK9.Url, "Pin Image") }
	} else { reply(client, chat, msg, "❌ Error") }
}

func dlYouTube(client *whatsmeow.Client, chat types.JID, url, format string, msg *waProto.Message) {
	react(client, chat, msg, "📺")
	type R struct { BK9 struct { Mp4 string `json:"mp4"`; Mp3 string `json:"mp3"` } `json:"BK9"`; Status bool `json:"status"` }
	var res R
	if getJson("https://bk9.fun/downloader/youtube?url="+url, &res) == nil && res.Status {
		if format == "mp4" && res.BK9.Mp4 != "" { sendVideo(client, chat, res.BK9.Mp4, "YT Video") }
		if format == "mp3" && res.BK9.Mp3 != "" { sendDoc(client, chat, res.BK9.Mp3, "audio.mp3", "audio/mpeg") }
	} else { reply(client, chat, msg, "❌ Error") }
}

// --- 🛠️ TOOLS & GROUPS ---
func makeSticker(client *whatsmeow.Client, chat types.JID, msg *waProto.Message) {
	react(client, chat, msg, "🎨")
	data, err := downloadMedia(client, msg)
	if err != nil { reply(client, chat, msg, "⚠️ Reply Image"); return }
	// FFmpeg conversion logic here (simplified)
	f := "temp.webp"; ioutil.WriteFile(f, data, 0644)
	exec.Command("ffmpeg", "-y", "-i", f, "-vcodec", "libwebp", "out.webp").Run()
	d, _ := ioutil.ReadFile("out.webp")
	up, _ := client.Upload(context.Background(), d, whatsmeow.MediaImage)
	client.SendMessage(context.Background(), chat, &waProto.Message{StickerMessage: &waProto.StickerMessage{
		Url: proto.String(up.URL), DirectPath: proto.String(up.DirectPath),
		MediaKey: up.MediaKey, FileEncSha256: up.FileEncSHA256, FileSha256: up.FileSHA256,
		Mimetype: proto.String("image/webp"),
	}})
}

func removeBG(client *whatsmeow.Client, chat types.JID, msg *waProto.Message) {
	react(client, chat, msg, "✂️")
	data, err := downloadMedia(client, msg)
	if err != nil { return }
	url := uploadToCatbox(data)
	sendImage(client, chat, "https://bk9.fun/tools/removebg?url="+url, "✂️ Removed")
}

func stickerToImg(client *whatsmeow.Client, chat types.JID, msg *waProto.Message) {
	react(client, chat, msg, "🖼️")
	data, err := downloadMedia(client, msg)
	if err != nil { return }
	// Convert logic
	ioutil.WriteFile("in.webp", data, 0644)
	exec.Command("ffmpeg", "-y", "-i", "in.webp", "out.png").Run()
	d, _ := ioutil.ReadFile("out.png")
	up, _ := client.Upload(context.Background(), d, whatsmeow.MediaImage)
	client.SendMessage(context.Background(), chat, &waProto.Message{ImageMessage: &waProto.ImageMessage{
		Url: proto.String(up.URL), DirectPath: proto.String(up.DirectPath), MediaKey: up.MediaKey,
		FileEncSha256: up.FileEncSHA256, FileSha256: up.FileSHA256, Mimetype: proto.String("image/png"),
	}})
}

func reminiEnhance(client *whatsmeow.Client, chat types.JID, msg *waProto.Message) {
	react(client, chat, msg, "✨")
	data, err := downloadMedia(client, msg)
	if err != nil { return }
	url := uploadToCatbox(data)
	type R struct { Url string `json:"url"` }
	var res R
	if getJson("https://remini.mobilz.pw/enhance?url="+url, &res) == nil && res.Url != "" { sendImage(client, chat, res.Url, "Enhanced") }
}

func groupAction(client *whatsmeow.Client, chat types.JID, msg *waProto.Message, action string, isGroup bool) {
	if !isGroup { return }
	target := getTarget(msg)
	if target == nil { reply(client, chat, msg, "⚠️ Reply/Tag"); return }
	var c whatsmeow.ParticipantChange
	if action == "remove" { c = whatsmeow.ParticipantChangeRemove }
	if action == "promote" { c = whatsmeow.ParticipantChangePromote }
	if action == "demote" { c = whatsmeow.ParticipantChangeDemote }
	client.UpdateGroupParticipants(context.Background(), chat, []types.JID{*target}, c)
	reply(client, chat, msg, "✅ Done")
}

func groupTagAll(client *whatsmeow.Client, chat types.JID, text string, isGroup bool) {
	if !isGroup { return }
	info, _ := client.GetGroupInfo(context.Background(), chat)
	mentions := []string{}
	out := "📣 *TAG ALL*\n" + text + "\n"
	for _, p := range info.Participants { mentions = append(mentions, p.JID.String()); out += "@" + p.JID.User + "\n" }
	client.SendMessage(context.Background(), chat, &waProto.Message{ExtendedTextMessage: &waProto.ExtendedTextMessage{
		Text: proto.String(out), ContextInfo: &waProto.ContextInfo{MentionedJID: mentions},
	}})
}

func groupHideTag(client *whatsmeow.Client, chat types.JID, text string, isGroup bool) {
	if !isGroup { return }
	info, _ := client.GetGroupInfo(context.Background(), chat)
	mentions := []string{}
	for _, p := range info.Participants { mentions = append(mentions, p.JID.String()) }
	client.SendMessage(context.Background(), chat, &waProto.Message{ExtendedTextMessage: &waProto.ExtendedTextMessage{
		Text: proto.String(text), ContextInfo: &waProto.ContextInfo{MentionedJID: mentions},
	}})
}

func groupOpenClose(client *whatsmeow.Client, chat types.JID, args []string, isGroup bool) {
	if !isGroup || len(args) == 0 { return }
	announce := false; if args[0] == "close" { announce = true }
	client.SetGroupAnnounce(context.Background(), chat, announce)
	reply(client, chat, nil, "✅ Group Settings Updated")
}

func groupAdd(client *whatsmeow.Client, chat types.JID, args []string, isGroup bool) {
	if !isGroup || len(args) == 0 { return }
	jid, _ := types.ParseJID(args[0] + "@s.whatsapp.net")
	client.UpdateGroupParticipants(context.Background(), chat, []types.JID{jid}, whatsmeow.ParticipantChangeAdd)
}

func deleteMsg(client *whatsmeow.Client, chat types.JID, msg *waProto.Message) {
	if msg.ExtendedTextMessage == nil { return }
	ctx := msg.ExtendedTextMessage.ContextInfo
	if ctx == nil { return }
	target, _ := types.ParseJID(*ctx.Participant)
	client.RevokeMessage(context.Background(), chat, target, *ctx.StanzaID)
}

// --- ⚙️ HELPERS & UTILS ---
func loadData() {
	b, _ := ioutil.ReadFile(dataFile)
	json.Unmarshal(b, &data)
	if data.Settings == nil { data.Settings = make(map[string]*GroupSettings) }
	if data.Prefix == "" { data.Prefix = "#" }
}
func saveData() {
	b, _ := json.MarshalIndent(data, "", "  ")
	ioutil.WriteFile(dataFile, b, 0644)
}
func getGroupSettings(id string) *GroupSettings {
	dataMutex.Lock(); defer dataMutex.Unlock()
	if data.Settings[id] == nil { data.Settings[id] = &GroupSettings{Mode: "public", AntilinkAdmin: true, AntilinkAction: "delete", Warnings: make(map[string]int)} }
	return data.Settings[id]
}
func makeCard(title, body string) string { return fmt.Sprintf("╭━━━〔 *%s* 〕━━━┈\n┃ %s\n╰━━━━━━━━━━━━━━━━━━┈", title, body) }
func reply(client *whatsmeow.Client, chat types.JID, q *waProto.Message, text string) {
	client.SendMessage(context.Background(), chat, &waProto.Message{ExtendedTextMessage: &waProto.ExtendedTextMessage{
		Text: proto.String(text), ContextInfo: &waProto.ContextInfo{StanzaID: proto.String(q.GetKey().GetId()), Participant: proto.String(q.GetKey().GetParticipant()), QuotedMessage: q},
	}})
}
func react(client *whatsmeow.Client, chat types.JID, q *waProto.Message, e string) {
	if q == nil { return }
	client.SendMessage(context.Background(), chat, &waProto.Message{ReactionMessage: &waProto.ReactionMessage{Key: q.Key, Text: proto.String(e)}})
}
func getText(m *waProto.Message) string {
	if m.Conversation != nil { return *m.Conversation }
	if m.ExtendedTextMessage != nil { return *m.ExtendedTextMessage.Text }
	if m.ImageMessage != nil { return *m.ImageMessage.Caption }
	return ""
}
func isAdmin(client *whatsmeow.Client, chat, user types.JID) bool {
	info, _ := client.GetGroupInfo(context.Background(), chat)
	for _, p := range info.Participants { if p.JID.User == user.User && (p.IsAdmin || p.IsSuperAdmin) { return true } }
	return false
}
func isOwner(client *whatsmeow.Client, user types.JID) bool { return user.User == client.Store.ID.User || user.User == OWNER_NUMBER }
func canExecute(client *whatsmeow.Client, v *events.Message, cmd string) bool {
	if isOwner(client, v.Info.Sender) { return true }
	s := getGroupSettings(v.Info.Chat.String())
	if s.Mode == "private" { return false }
	if s.Mode == "admin" { return isAdmin(client, v.Info.Chat, v.Info.Sender) }
	return true
}
func getTarget(m *waProto.Message) *types.JID {
	if m.ExtendedTextMessage == nil { return nil }
	c := m.ExtendedTextMessage.ContextInfo
	if c == nil { return nil }
	if len(c.MentionedJID) > 0 { j, _ := types.ParseJID(c.MentionedJID[0]); return &j }
	if c.Participant != nil { j, _ := types.ParseJID(*c.Participant); return &j }
	return nil
}
func getJson(url string, t interface{}) error { r, err := http.Get(url); if err!=nil { return err }; defer r.Body.Close(); return json.NewDecoder(r.Body).Decode(t) }
func downloadMedia(client *whatsmeow.Client, m *waProto.Message) ([]byte, error) {
	var d *waProto.ImageMessage
	if m.ImageMessage != nil { d = m.ImageMessage }
	if m.ExtendedTextMessage != nil && m.ExtendedTextMessage.ContextInfo != nil {
		q := m.ExtendedTextMessage.ContextInfo.QuotedMessage
		if q != nil && q.ImageMessage != nil { d = q.ImageMessage }
		if q != nil && q.StickerMessage != nil { return client.DownloadAny(q.StickerMessage) }
	}
	if d == nil { return nil, fmt.Errorf("no media") }
	return client.Download(d)
}
func uploadToCatbox(d []byte) string {
	b := new(bytes.Buffer); w := multipart.NewWriter(b); p, _ := w.CreateFormFile("fileToUpload", "f.jpg"); p.Write(d); w.WriteField("reqtype", "fileupload"); w.Close()
	r, _ := http.Post("https://catbox.moe/user/api.php", w.FormDataContentType(), b); res, _ := ioutil.ReadAll(r.Body); return string(res)
}
func sendVideo(client *whatsmeow.Client, chat types.JID, url, cap string) {
	r, _ := http.Get(url); d, _ := ioutil.ReadAll(r.Body)
	up, _ := client.Upload(context.Background(), d, whatsmeow.MediaVideo)
	client.SendMessage(context.Background(), chat, &waProto.Message{VideoMessage: &waProto.VideoMessage{
		Url: proto.String(up.URL), DirectPath: proto.String(up.DirectPath), MediaKey: up.MediaKey,
		FileEncSha256: up.FileEncSHA256, FileSha256: up.FileSHA256, Mimetype: proto.String("video/mp4"), Caption: proto.String(cap),
	}})
}
func sendImage(client *whatsmeow.Client, chat types.JID, url, cap string) {
	r, _ := http.Get(url); d, _ := ioutil.ReadAll(r.Body)
	up, _ := client.Upload(context.Background(), d, whatsmeow.MediaImage)
	client.SendMessage(context.Background(), chat, &waProto.Message{ImageMessage: &waProto.ImageMessage{
		Url: proto.String(up.URL), DirectPath: proto.String(up.DirectPath), MediaKey: up.MediaKey,
		FileEncSha256: up.FileEncSHA256, FileSha256: up.FileSHA256, Mimetype: proto.String("image/jpeg"), Caption: proto.String(cap),
	}})
}
func sendDoc(client *whatsmeow.Client, chat types.JID, url, n, m string) {
	r, _ := http.Get(url); d, _ := ioutil.ReadAll(r.Body)
	up, _ := client.Upload(context.Background(), d, whatsmeow.MediaDocument)
	client.SendMessage(context.Background(), chat, &waProto.Message{DocumentMessage: &waProto.DocumentMessage{
		Url: proto.String(up.URL), DirectPath: proto.String(up.DirectPath), MediaKey: up.MediaKey,
		FileEncSha256: up.FileEncSHA256, FileSha256: up.FileSHA256, Mimetype: proto.String(m), FileName: proto.String(n),
	}})
}
func sendPing(client *whatsmeow.Client, chat types.JID, msg *waProto.Message) {
	react(client, chat, msg, "⚡"); s := time.Now(); reply(client, chat, msg, "🏓 Pinging...")
	reply(client, chat, msg, fmt.Sprintf("*⚡ Ping:* %dms", time.Since(s).Milliseconds()))
}
func getWeather(client *whatsmeow.Client, chat types.JID, city string, msg *waProto.Message) {
	react(client, chat, msg, "🌦️"); r, _ := http.Get("https://wttr.in/" + city + "?format=%C+%t"); d, _ := ioutil.ReadAll(r.Body)
	reply(client, chat, msg, fmt.Sprintf("🌦️ *%s:* %s", city, string(d)))
}
func doTranslate(client *whatsmeow.Client, chat types.JID, args []string, msg *waProto.Message) {
	react(client, chat, msg, "🌍"); txt := strings.Join(args, " ")
	if txt == "" { q := msg.ExtendedTextMessage.GetContextInfo().GetQuotedMessage(); if q != nil { txt = q.GetConversation() } }
	url := fmt.Sprintf("https://translate.googleapis.com/translate_a/single?client=gtx&sl=auto&tl=ur&dt=t&q=%s", url.QueryEscape(txt))
	r, _ := http.Get(url); var res []interface{}; json.NewDecoder(r.Body).Decode(&res)
	if len(res) > 0 { reply(client, chat, msg, "🌍 "+res[0].([]interface{})[0].([]interface{})[0].(string)) }
}
func mediaToUrl(client *whatsmeow.Client, chat types.JID, msg *waProto.Message) {
	d, err := downloadMedia(client, msg); if err != nil { return }; reply(client, chat, msg, "🔗 "+uploadToCatbox(d))
}