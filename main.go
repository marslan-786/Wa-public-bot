package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
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
	dbLog := waLog.Stdout("Database", "INFO", true)
	// PostgreSQL سیشن کے لیے
	container, err := sqlstore.New(context.Background(), "postgres", os.Getenv("DATABASE_URL"), dbLog)
	if err != nil { panic(err) }

	deviceStore, err := container.GetFirstDevice(context.Background())
	if err != nil { panic(err) }

	client = whatsmeow.NewClient(deviceStore, waLog.Stdout("Client", "INFO", true))
	client.AddEventHandler(eventHandler)

	// ریلوے پورٹ مینجمنٹ [Railway Port Handling]
	port := getEnv("PORT", "8080")
	r := gin.Default()
	r.StaticFile("/", "./web/index.html")
	r.StaticFile("/pic.png", "./web/pic.png")

	r.POST("/api/pair", func(c *gin.Context) {
		var req struct{ Number string `json:"number"` }
		if err := c.BindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": "Invalid number"})
			return
		}
		if client.Store.ID == nil {
			client.Connect()
			code, err := client.PairPhone(context.Background(), req.Number, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
			if err != nil {
				c.JSON(500, gin.H{"error": err.Error()})
				return
			}
			c.JSON(200, gin.H{"code": code})
		}
	})

	go r.Run(":" + port)

	if client.Store.ID != nil {
		client.Connect()
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	client.Disconnect()
}

// واٹس ایپ 3-Line مینیو بٹن (نئی لائبریری کے مطابق)
func sendOfficialMenu(chat types.JID) {
	listMsg := &waProto.ListMessage{
		Title:       proto.String("IMPOSSIBLE MENU"),
		Description: proto.String("Select a category"),
		ButtonText:  proto.String("MENU"), // آپ کی فرمائش پر نام "MENU"
		ListType:    waProto.ListMessage_SINGLE_SELECT.Enum(),
		Sections: []*waProto.ListMessage_Section{
			{
				Title: proto.String("ADMIN TOOLS"),
				Rows: []*waProto.ListMessage_Row{
					{Title: proto.String("Kick"), RowID: proto.String("kick")},
					{Title: proto.String("Add"), RowID: proto.String("add")},
				},
			},
			{
				Title: proto.String("DOWNLOADERS"),
				Rows: []*waProto.ListMessage_Row{
					{Title: proto.String("Instagram"), RowID: proto.String("ig")},
					{Title: proto.String("TikTok"), RowID: proto.String("tt")},
				},
			},
		},
	}
	client.SendMessage(context.Background(), chat, &waProto.Message{ListMessage: listMsg})
}

func eventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		if v.Message.GetConversation() == "#menu" {
			sendOfficialMenu(v.Info.Chat)
		}
	}
}