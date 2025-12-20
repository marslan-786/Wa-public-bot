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

var (
	client    *whatsmeow.Client
	container *sqlstore.Container
)

const (
	BOT_JID_USER = "impossible-bot-v1"
	DEVELOPER    = "Nothing Is Impossible"
)

func main() {
	fmt.Println("🚀 Impossible Bot starting (Railway + Postgres Safe)")

	ctx := context.Background()

	dbURL := os.Getenv("DATABASE_URL")
	dbType := "postgres"
	if dbURL == "" {
		dbType = "sqlite3"
		dbURL = "file:impossible.db?_foreign_keys=on"
	}

	var err error
	container, err = sqlstore.New(ctx, dbType, dbURL, waLog.Stdout("DB", "INFO", true))
	if err != nil {
		panic(err)
	}

	// ✅ Stable device identity
	var device *store.Device
	devices, _ := container.GetAllDevices(ctx)

	for _, d := range devices {
		if d.ID != nil && d.ID.User == BOT_JID_USER {
			device = d
			break
		}
	}

	if device == nil {
		device = container.NewDevice()
		device.ID = &types.JID{
			User:   BOT_JID_USER,
			Server: "s.whatsapp.net",
		}
	}

	client = whatsmeow.NewClient(device, waLog.Stdout("Client", "INFO", true))
	client.AddEventHandler(eventHandler)

	// ✅ Always connect (WhatsMeow handles restore)
	go client.Connect()

	// ---------- HTTP ----------
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	r.POST("/api/pair", handlePairAPI)
	go r.Run(":" + port)

	// ---------- Shutdown ----------
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	<-ch

	client.Disconnect()
}

// ---------- MESSAGE ----------

func getBody(msg *waProto.Message) string {
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

func eventHandler(evt interface{}) {
	switch v := evt.(type) {

	case *events.Message:
		if v.Info.IsFromMe {
			return
		}

		body := strings.TrimSpace(strings.ToLower(getBody(v.Message)))
		fmt.Println("📩", v.Info.Sender.User, body)

		switch body {
		case "#menu":
			sendOfficialListMenu(v.Info.Chat)

		case "#ping":
			start := time.Now()
			latency := time.Since(start)
			text := fmt.Sprintf(
				"🚀 *IMPOSSIBLE PING*\n\nLatency: `%s`\nDev: _%s_",
				latency,
				DEVELOPER,
			)
			client.SendMessage(context.Background(), v.Info.Chat,
				&waProto.Message{Conversation: proto.String(text)})
		}
	}
}

// ---------- LIST MENU ----------

func sendOfficialListMenu(chat types.JID) {
	list := &waProto.ListMessage{
		Title:       proto.String("IMPOSSIBLE MENU"),
		Description: proto.String("نیچے بٹن دبائیں 👇"),
		ButtonText:  proto.String("Open Menu"),
		ListType:    waProto.ListMessage_SINGLE_SELECT.Enum(),
		Sections: []*waProto.ListMessage_Section{
			{
				Title: proto.String("BOT FEATURES"),
				Rows: []*waProto.ListMessage_Row{
					{
						RowID:       proto.String("ping_row"),
						Title:       proto.String("Check Speed"),
						Description: proto.String("Server latency"),
					},
					{
						RowID:       proto.String("id_row"),
						Title:       proto.String("User Info"),
						Description: proto.String("Your JID details"),
					},
				},
			},
		},
	}

	_, err := client.SendMessage(context.Background(), chat, &waProto.Message{
		ListMessage: list,
		ContextInfo: &waProto.ContextInfo{},
	})

	if err != nil {
		client.SendMessage(context.Background(), chat,
			&waProto.Message{Conversation: proto.String("❌ Menu unavailable")})
	}
}

// ---------- PAIR API ----------

func handlePairAPI(c *gin.Context) {
	var req struct {
		Number string `json:"number"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}

	num := strings.ReplaceAll(req.Number, "+", "")

	code, err := client.PairPhone(
		context.Background(),
		num,
		true,
		whatsmeow.PairClientChrome,
		"Chrome (Linux)",
	)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"code": code})
}