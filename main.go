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

func main() {
	fmt.Println("🚀 Starting Impossible Bot Engine...")

	// ڈیٹا بیس سیٹ اپ
	dbURL := os.Getenv("DATABASE_URL")
	dbType := "postgres"
	if dbURL == "" {
		dbURL = "file:impossible_session.db?_foreign_keys=on"
		dbType = "sqlite3"
	}

	dbLog := waLog.Stdout("Database", "INFO", true)
	container, err := sqlstore.New(context.Background(), dbType, dbURL, dbLog)
	if err != nil { panic(err) }

	deviceStore, err := container.GetFirstDevice(context.Background())
	if err != nil { panic(err) }

	client = whatsmeow.NewClient(deviceStore, waLog.Stdout("Client", "INFO", true))
	client.AddEventHandler(eventHandler)

	// ویب سرور
	port := os.Getenv("PORT")
	if port == "" { port = "8080" }
	
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	r.StaticFile("/", "./web/index.html")
	r.StaticFile("/pic.png", "./web/pic.png")

	r.POST("/api/pair", func(c *gin.Context) {
		var req struct{ Number string `json:"number"` }
		if err := c.BindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": "Invalid input"})
			return
		}

		// کنکشن گارڈ: اگر کنیکٹ نہیں ہے تو کریں
		if !client.IsConnected() {
			client.Connect()
			// واٹس ایپ کو ریڈی ہونے کے لیے وقت دیں
			time.Sleep(5 * time.Second) 
		}

		// پیرنگ کوڈ جنریٹ کرنے کی کوشش (بشمول ری ٹرائی لاجک)
		var code string
		var pairErr error
		for i := 0; i < 3; i++ { // 3 بار کوشش کریں
			code, pairErr = client.PairPhone(context.Background(), req.Number, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
			if pairErr == nil {
				break
			}
			fmt.Printf("⚠️ Retrying Pairing... (%d/3)\n", i+1)
			time.Sleep(3 * time.Second)
		}

		if pairErr != nil {
			c.JSON(500, gin.H{"error": "Connection unstable. Please refresh page and try again."})
			return
		}

		c.JSON(200, gin.H{"code": code})
	})

	go func() {
		fmt.Printf("🌐 Web Dashboard live on port %s\n", port)
		r.Run(":" + port)
	}()

	if client.Store.ID != nil {
		client.Connect()
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	client.Disconnect()
}

func eventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		body := v.Message.GetConversation()
		if body == "" {
			body = v.Message.GetExtendedTextMessage().GetText()
		}

		if strings.TrimSpace(body) == "#menu" {
			sendOfficialMenu(v.Info.Chat)
		}
	}
}

func sendOfficialMenu(chat types.JID) {
	// واٹس ایپ MENU بٹن (List Message)
	listMsg := &waProto.ListMessage{
		Title:       proto.String("IMPOSSIBLE BOT MENU"),
		Description: proto.String("Select a command category below"),
		ButtonText:  proto.String("MENU"),
		ListType:    waProto.ListMessage_SINGLE_SELECT.Enum(),
		Sections: []*waProto.ListMessage_Section{
			{
				Title: proto.String("AVAILABLE TOOLS"),
				Rows: []*waProto.ListMessage_Row{
					{Title: proto.String("Ping Status"), RowID: proto.String("ping")},
					{Title: proto.String("Check ID"), RowID: proto.String("id")},
				},
			},
		},
	}
	client.SendMessage(context.Background(), chat, &waProto.Message{ListMessage: listMsg})
}