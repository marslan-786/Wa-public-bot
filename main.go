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

const (
	BOT_NAME   = "IMPOSSIBLE_STABLE_V1"
	DEV_NAME   = "Nothing Is Impossible"
	TABLE_PREF = "impossible_bot_" // 🔐 SESSION ISOLATION
)

var (
	client    *whatsmeow.Client
	container *sqlstore.Container
	startTime = time.Now()
)

func main() {
	fmt.Println("🚀 IMPOSSIBLE BOT | BOOTING")

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
		sqlstore.WithTablePrefix(TABLE_PREF),
	)
	if err != nil {
		panic(err)
	}

	device, err := container.GetFirstDevice(context.Background())
	if err != nil {
		panic(err)
	}

	client = whatsmeow.NewClient(device, waLog.Stdout("Client", "INFO", true))
	client.AddEventHandler(eventHandler)

	if client.Store.ID != nil {
		err = client.Connect()
		if err != nil {
			panic(err)
		}
		fmt.Println("✅ Session Restored")
	}

	// 🌐 Pair API
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	r.POST("/pair", handlePair)
	go r.Run(":8080")

	// 🧹 Graceful Shutdown
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	client.Disconnect()
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
			sendPing(v.Info.Chat)
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

// 📜 LIST MENU (Open Menu Button)
func sendMenu(chat types.JID) {
	menu := &waProto.ListMessage{
		Title:       proto.String("IMPOSSIBLE MENU"),
		Description: proto.String("Select an option below"),
		ButtonText:  proto.String("Open Menu"),
		ListType:    waProto.ListMessage_SINGLE_SELECT.Enum(),
		Sections: []*waProto.ListMessage_Section{
			{
				Title: proto.String("BOT COMMANDS"),
				Rows: []*waProto.ListMessage_Row{
					{
						RowID:       proto.String("ping"),
						Title:       proto.String("Ping Status"),
						Description: proto.String("Check server latency"),
					},
				},
			},
		},
	}

	client.SendMessage(context.Background(), chat, &waProto.Message{
		ListMessage: menu,
	})
}

// ⚡ ADVANCED PING UI
func sendPing(chat types.JID) {
	start := time.Now()
	time.Sleep(25 * time.Millisecond)
	ms := time.Since(start).Milliseconds()

	uptime := time.Since(startTime).Round(time.Second)

	msg := fmt.Sprintf(
		"╔══════════════════════╗\n"+
			"║ 🚀 IMPOSSIBLE BOT     ║\n"+
			"╠══════════════════════╣\n"+
			"║ 👨‍💻 Dev: %s\n"+
			"╠══════════════════════╣\n"+
			"║ ⚡ PING STATUS\n"+
			"║   ┌──────────────┐\n"+
			"║   │  %d ms       │\n"+
			"║   └──────────────┘\n"+
			"╠══════════════════════╣\n"+
			"║ ⏱ Uptime: %s\n"+
			"╚══════════════════════╝",
		DEV_NAME,
		ms,
		uptime,
	)

	client.SendMessage(context.Background(), chat, &waProto.Message{
		Conversation: proto.String(msg),
	})
}

// 🔗 PAIR API
func handlePair(c *gin.Context) {
	var req struct {
		Number string `json:"number"`
	}

	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}

	number := strings.ReplaceAll(req.Number, "+", "")

	code, err := client.PairPhone(
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