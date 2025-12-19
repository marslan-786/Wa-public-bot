package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

var client *whatsmeow.Client
var container *sqlstore.Container

// سیشن آئسولیشن کے لیے مخصوص آئی ڈی
const BOT_IDENTITY = "impossible_menu_bot_v1"

func main() {
	fmt.Println("🚀 [Impossible Bot] Booting with Session Isolation...")

	dbURL := os.Getenv("DATABASE_URL")
	dbType := "postgres"
	if dbURL == "" { dbType = "sqlite3"; dbURL = "file:impossible.db?_foreign_keys=on" }

	var err error
	container, err = sqlstore.New(context.Background(), dbType, dbURL, waLog.Stdout("Database", "INFO", true))
	if err != nil { panic(err) }

	// سیشن آئسولیشن: ہم پہلا سیشن نہیں اٹھائیں گے، بلکہ اس بوٹ کا مخصوص سیشن ڈھونڈیں گے
	deviceStore, err := container.GetDeviceByJID(types.NewJID(BOT_IDENTITY, types.DefaultUserServer))
	if err != nil || deviceStore == nil {
		fmt.Println("ℹ️ [Auth] No dedicated session found. Creating a fresh one for this bot identity.")
		deviceStore = container.NewDevice()
	}

	client = whatsmeow.NewClient(deviceStore, waLog.Stdout("Client", "INFO", true))
	client.AddEventHandler(eventHandler)

	// اگر پہلے سے لاگ ان ہے تو کنیکٹ کریں
	if client.Store.ID != nil {
		fmt.Printf("✅ [Status] Logged in as: %s. Connecting...\n", client.Store.ID.User)
		err := client.Connect()
		if err != nil { fmt.Printf("❌ Connection Failed: %v\n", err) }
	}

	port := os.Getenv("PORT")
	if port == "" { port = "8080" }
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	r.StaticFile("/", "./web/index.html")
	r.StaticFile("/pic.png", "./web/pic.png")
	r.POST("/api/pair", handlePairAPI)

	go r.Run(":" + port)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	client.Disconnect()
}

func getBody(msg *waProto.Message) string {
	if msg == nil { return "" }
	if msg.Conversation != nil { return msg.GetConversation() }
	if msg.ExtendedTextMessage != nil { return msg.ExtendedTextMessage.GetText() }
	if msg.ImageMessage != nil { return msg.ImageMessage.GetCaption() }
	return ""
}

func eventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		if v.Info.IsFromMe { return }
		body := strings.TrimSpace(getBody(v.Message))
		
		fmt.Printf("📩 [MSG] From: %s | Text: %s\n", v.Info.Sender.User, body)

		if strings.ToLower(body) == "#menu" {
			_, _ = client.SendMessage(context.Background(), v.Info.Chat, client.BuildReaction(v.Info.Chat, v.Info.Sender, v.Info.ID, "📜"))
			sendAdvancedMenu(v.Info.Chat)
		}
	}
}

func sendAdvancedMenu(chat types.JID) {
	fmt.Println("🖼️ [Menu] Processing Image and Interactive Buttons...")
	imgData, _ := os.ReadFile("./web/pic.png")
	uploadResp, _ := client.Upload(context.Background(), imgData, whatsmeow.MediaImage)

	// 1. پہلے تصویر بھیجیں
	imageMsg := &waProto.ImageMessage{
		Mimetype:      proto.String("image/png"),
		Caption:       proto.String("*📜 IMPOSSIBLE MENU*\n\nPowered by Go Engine"),
		URL:           &uploadResp.URL,
		DirectPath:    &uploadResp.DirectPath,
		MediaKey:      uploadResp.MediaKey,
		FileEncSHA256: uploadResp.FileEncSHA256,
		FileSHA256:    uploadResp.FileSHA256,
		FileLength:    proto.Uint64(uint64(len(imgData))),
	}
	client.SendMessage(context.Background(), chat, &waProto.Message{ImageMessage: imageMsg})

	// 2. اب تھری لائن بٹن (List Message) - نئے فارمیٹ میں
	listMsg := &waProto.ListMessage{
		Title:       proto.String("COMMAND CATEGORIES"),
		Description: proto.String("Click 'MENU' to explore all commands"),
		ButtonText:  proto.String("MENU"),
		ListType:    waProto.ListMessage_SINGLE_SELECT.Enum(),
		Sections: []*waProto.ListMessage_Section{
			{
				Title: proto.String("GENERAL TOOLS"),
				Rows: []*waProto.ListMessage_Row{
					{Title: proto.String("Bot Speed"), RowID: proto.String("ping"), Description: proto.String("Check latency")},
					{Title: proto.String("User Info"), RowID: proto.String("id")},
				},
			},
		},
	}

	fmt.Println("📤 Sending List Component...")
	_, err := client.SendMessage(context.Background(), chat, &waProto.Message{ListMessage: listMsg})
	if err != nil {
		fmt.Printf("❌ Button Failed: %v. Sending Text Fallback.\n", err)
		client.SendMessage(context.Background(), chat, &waProto.Message{Conversation: proto.String("⚠️ Your WhatsApp doesn't support buttons. Use commands like #ping, #id manually.")})
	}
}

func handlePairAPI(c *gin.Context) {
	var req struct{ Number string `json:"number"` }
	c.BindJSON(&req)
	num := strings.ReplaceAll(req.Number, "+", "")

	fmt.Printf("🧹 [Cleanup] Wiping any glitched data for identity: %s\n", BOT_IDENTITY)
	
	// ہم اس مخصوص identity کو صاف کر رہے ہیں
	devices, _ := container.GetAllDevices(context.Background())
	for _, dev := range devices {
		if dev.ID != nil && strings.Contains(dev.ID.User, num) {
			container.DeleteDevice(context.Background(), dev)
		}
	}

	newDevice := container.NewDevice()
	if client.IsConnected() { client.Disconnect() }
	client = whatsmeow.NewClient(newDevice, waLog.Stdout("Client", "INFO", true))
	client.AddEventHandler(eventHandler)
	client.Connect()
	
	time.Sleep(10 * time.Second)
	code, err := client.PairPhone(context.Background(), num, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
	
	if err != nil {
		fmt.Printf("❌ Pairing Failed: %v\n", err)
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	
	fmt.Printf("✅ Fresh Pairing Code Generated: %s\n", code)
	c.JSON(200, gin.H{"code": code})
}