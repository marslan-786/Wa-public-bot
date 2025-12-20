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
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"

	"google.golang.org/protobuf/proto"
)

const (
	BOT_TAG   = "IMPOSSIBLE_STABLE_V1"
	DEVELOPER = "Nothing Is Impossible"
)

var (
	startTime = time.Now()
	container *sqlstore.Container
	clients   []*whatsmeow.Client
)

func main() {
	fmt.Println("🚀 IMPOSSIBLE BOT | Starting")

	dbURL := os.Getenv("DATABASE_URL")
	dbType := "postgres"
	if dbURL == "" {
		dbType = "sqlite3"
		dbURL = "file:impossible.db?_foreign_keys=on"
	}

	var err error
	container, err = sqlstore.New(
		context.Background(),
		dbType,
		dbURL,
		waLog.Stdout("DB", "INFO", true),
	)
	if err != nil {
		panic(err)
	}

	// 🔒 SESSION ISOLATION (صرف اپنا BOT_TAG)
	devices, _ := container.GetAllDevices(context.Background())
	for _, dev := range devices {
		if dev.PushName != BOT_TAG {
			continue
		}

		cl := whatsmeow.NewClient(dev, waLog.Stdout("Client", "INFO", true))
		cl.AddEventHandler(eventHandler)
		clients = append(clients, cl)

		go func(c *whatsmeow.Client) {
			err := c.Connect()
			if err != nil {
				fmt.Println("❌ Connect failed:", err)
			} else {
				fmt.Println("✅ Session restored:", c.Store.ID)
			}
		}(cl)
	}

	// اگر پہلا run ہے
	if len(clients) == 0 {
		dev := container.NewDevice()
		dev.PushName = BOT_TAG
		cl := whatsmeow.NewClient(dev, waLog.Stdout("Client", "INFO", true))
		cl.AddEventHandler(eventHandler)
		clients = append(clients, cl)
	}

	// 🌐 Pair API
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	r.POST("/api/pair", handlePair)

	go r.Run(":8080")

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig

	for _, c := range clients {
		c.Disconnect()
	}
}

func eventHandler(evt interface{}) {
	switch v := evt.(type) {

	case *events.Message:
		if v.Info.IsFromMe {
			return
		}

		text := strings.ToLower(strings.TrimSpace(getText(v.Message)))

		switch text {
		case "#menu":
			sendMenu(v.Info.Chat)

		case "#ping":
			sendAdvancedPing(v.Info.Chat)
		}
	}
}

func getText(msg *waProto.Message) string {
	if msg == nil {
		return ""
	}
	if msg.Conversation != nil {
		return msg.GetConversation()
	}
	if msg.ExtendedTextMessage != nil {
		return msg.ExtendedTextMessage.GetText()
	}
	return ""
}

func sendMenu(chat types.JID) {
	menu := &waProto.ListMessage{
		Title:       proto.String("IMPOSSIBLE MENU"),
		Description: proto.String("Choose an option below"),
		ButtonText:  proto.String("Open Menu"),
		ListType:    waProto.ListMessage_SINGLE_SELECT.Enum(),
		Sections: []*waProto.ListMessage_Section{
			{
				Title: proto.String("FEATURES"),
				Rows: []*waProto.ListMessage_Row{
					{
						RowID:       proto.String("ping"),
						Title:       proto.String("Ping Status"),
						Description: proto.String("Check bot latency"),
					},
				},
			},
		},
	}

	clients[0].SendMessage(context.Background(), chat, &waProto.Message{
		ListMessage: menu,
	})
}

func sendAdvancedPing(chat types.JID) {
	start := time.Now()
	time.Sleep(30 * time.Millisecond) // simulate real latency
	elapsed := time.Since(start)

	// صرف short ms (dot کے بعد نہیں)
	ping := elapsed.Milliseconds()

	uptime := time.Since(startTime).Round(time.Second)

	msg := fmt.Sprintf(
		"╔══════════════════════╗\n"+
			"║   🚀 IMPOSSIBLE BOT   ║\n"+
			"╠══════════════════════╣\n"+
			"║ 👨‍💻 Dev: %s\n"+
			"╠══════════════════════╣\n"+
			"║   ⚡ PING STATUS\n"+
			"║   ┌──────────────┐\n"+
			"║   │  %d ms       │\n"+
			"║   └──────────────┘\n"+
			"╠══════════════════════╣\n"+
			"║ ⏱ Uptime: %s\n"+
			"╚══════════════════════╝",
		DEVELOPER,
		ping,
		uptime,
	)

	clients[0].SendMessage(context.Background(), chat, &waProto.Message{
		Conversation: proto.String(msg),
	})
}

func handlePair(c *gin.Context) {
	var req struct {
		Number string `json:"number"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}

	number := strings.ReplaceAll(req.Number, "+", "")
	cl := clients[0]

	code, err := cl.PairPhone(
		context.Background(),
		number,
		true,
		whatsmeow.PairClientChrome,
		"Chrome Linux",
	)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"code": code})
}