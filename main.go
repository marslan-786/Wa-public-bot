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

// اس بوٹ کی منفرد شناخت
const BOT_TAG = "IMPOSSIBLE_MENU_BOT"

func main() {
	fmt.Println("🚀 [System] Impossible Bot: Starting Text + Button Mode...")

	dbURL := os.Getenv("DATABASE_URL")
	dbType := "postgres"
	if dbURL == "" { dbType = "sqlite3"; dbURL = "file:impossible.db?_foreign_keys=on" }

	var err error
	container, err = sqlstore.New(context.Background(), dbType, dbURL, waLog.Stdout("Database", "INFO", true))
	if err != nil { panic(err) }

	// صرف اپنا مخصوص سیشن تلاش کریں
	var targetDevice *sqlstore.Device
	devices, _ := container.GetAllDevices(context.Background())
	for _, dev := range devices {
		if dev.PushName == BOT_TAG {
			targetDevice = dev
			break
		}
	}

	if targetDevice == nil {
		fmt.Println("ℹ️ [Auth] Bot is IDLE. Waiting for login from Web Dashboard.")
		targetDevice = container.NewDevice()
		targetDevice.PushName = BOT_TAG
	}

	client = whatsmeow.NewClient(targetDevice, waLog.Stdout("Client", "INFO", true))
	client.AddEventHandler(eventHandler)

	if client.Store.ID != nil {
		fmt.Printf("✅ [Status] Logged in as: %s\n", client.Store.ID.User)
		client.Connect()
	}

	port := os.Getenv("PORT")
	if port == "" { port = "8080" }
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	r.StaticFile("/", "./web/index.html")
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
	return ""
}

func eventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		if v.Info.IsFromMe { return }
		body := strings.TrimSpace(getBody(v.Message))
		
		fmt.Printf("📩 [MSG] From: %s | Text: %s\n", v.Info.Sender.User, body)

		if strings.ToLower(body) == "#menu" {
			// ری ایکشن دیں
			_, _ = client.SendMessage(context.Background(), v.Info.Chat, client.BuildReaction(v.Info.Chat, v.Info.Sender, v.Info.ID, "📜"))
			sendButtonMenu(v.Info.Chat)
		}
	}
}

func sendButtonMenu(chat types.JID) {
	fmt.Println("📤 [Action] Sending Text-Only Button Menu...")

	// لسٹ مینیو سٹرکچر
	listMsg := &waProto.ListMessage{
		Title:       proto.String("IMPOSSIBLE MENU"),
		Description: proto.String("Hi! Select an option from the list below to use the bot tools."),
		ButtonText:  proto.String("CLICK TO OPEN"),
		ListType:    waProto.ListMessage_SINGLE_SELECT.Enum(),
		Sections: []*waProto.ListMessage_Section{
			{
				Title: proto.String("AVAILABLE TOOLS"),
				Rows: []*waProto.ListMessage_Row{
					{Title: proto.String("Ping Status"), RowID: proto.String("ping"), Description: proto.String("Check bot latency")},
					{Title: proto.String("My ID"), RowID: proto.String("id"), Description: proto.String("Get your WhatsApp JID")},
				},
			},
		},
	}

	// صرف ٹیکسٹ اور بٹن بھیجنا
	_, err := client.SendMessage(context.Background(), chat, &waProto.Message{
		ListMessage: listMsg,
	})

	if err != nil {
		fmt.Printf("❌ [Error] Button delivery failed: %v. Sending Text Fallback.\n", err)
		client.SendMessage(context.Background(), chat, &waProto.Message{
			Conversation: proto.String("*📜 IMPOSSIBLE MENU*\n\n• #ping\n• #id\n\n(Buttons are blocked on this account)"),
		})
	} else {
		fmt.Println("✅ [Success] Menu sent without image.")
	}
}

func handlePairAPI(c *gin.Context) {
	var req struct{ Number string `json:"number"` }
	c.BindJSON(&req)
	num := strings.ReplaceAll(req.Number, "+", "")

	fmt.Printf("🧹 [Cleanup] Wiping identity: %s for number: %s\n", BOT_TAG, num)
	
	devices, _ := container.GetAllDevices(context.Background())
	for _, dev := range devices {
		if dev.PushName == BOT_TAG {
			container.DeleteDevice(context.Background(), dev)
		}
	}

	newStore := container.NewDevice()
	newStore.PushName = BOT_TAG 

	if client.IsConnected() { client.Disconnect() }
	client = whatsmeow.NewClient(newStore, waLog.Stdout("Client", "INFO", true))
	client.AddEventHandler(eventHandler)
	client.Connect()
	
	time.Sleep(10 * time.Second)
	code, err := client.PairPhone(context.Background(), num, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"code": code})
}