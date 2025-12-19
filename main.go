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

func main() {
	fmt.Println("🚀 [Impossible Bot] Initializing Verbose Engine...")

	dbURL := os.Getenv("DATABASE_URL")
	dbType := "postgres"
	if dbURL == "" {
		fmt.Println("⚠️ [DB] No DATABASE_URL found, using local SQLite")
		dbURL = "file:impossible_session.db?_foreign_keys=on"
		dbType = "sqlite3"
	}

	dbLog := waLog.Stdout("Database", "INFO", true)
	var err error
	container, err = sqlstore.New(context.Background(), dbType, dbURL, dbLog)
	if err != nil { panic(err) }

	deviceStore, err := container.GetFirstDevice(context.Background())
	if err != nil { panic(err) }

	client = whatsmeow.NewClient(deviceStore, waLog.Stdout("Client", "INFO", true))
	client.AddEventHandler(eventHandler)

	port := os.Getenv("PORT")
	if port == "" { port = "8080" }

	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	r.StaticFile("/", "./web/index.html")
	r.StaticFile("/pic.png", "./web/pic.png")

	r.POST("/api/pair", handlePairAPI)

	go func() {
		fmt.Printf("🌐 [Server] Listening on port %s\n", port)
		r.Run(":" + port)
	}()

	if client.Store.ID != nil {
		fmt.Println("🔄 [System] Resuming previous session...")
		client.Connect()
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	client.Disconnect()
}

func getMessageText(msg *waProto.Message) string {
	if msg == nil { return "" }
	if msg.Conversation != nil { return msg.GetConversation() }
	if msg.ExtendedTextMessage != nil { return msg.ExtendedTextMessage.GetText() }
	if msg.ImageMessage != nil { return msg.ImageMessage.GetCaption() }
	if msg.VideoMessage != nil { return msg.VideoMessage.GetCaption() }
	return ""
}

func eventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		if v.Info.IsFromMe { return }

		body := strings.TrimSpace(getMessageText(v.Message))
		fmt.Printf("📩 [Log] Incoming Message | From: %s | Text: %s\n", v.Info.Sender.User, body)

		if body == "#menu" {
			fmt.Println("⚙️ [Action] Command #menu identified.")
			
			// ری ایکشن دیں
			_, _ = client.SendMessage(context.Background(), v.Info.Chat, client.BuildReaction(v.Info.Chat, v.Info.Sender, v.Info.ID, "📜"))
			
			sendMenuWithImage(v.Info.Chat)
		}
	}
}

func sendMenuWithImage(chat types.JID) {
	fmt.Println("🖼️ [Media] Reading pic.png from ./web/ folder...")
	imgData, err := os.ReadFile("./web/pic.png")
	if err != nil {
		fmt.Printf("❌ [File Error] Failed to read pic.png: %v\n", err)
		client.SendMessage(context.Background(), chat, &waProto.Message{Conversation: proto.String("*📜 MENU*\nImage file missing.")})
		return
	}

	// واٹس ایپ سرور پر تصویر اپلوڈ کرنا
	fmt.Println("📤 [Upload] Sending image to WhatsApp Media Servers...")
	uploadResp, err := client.Upload(context.Background(), imgData, whatsmeow.MediaImage)
	if err != nil {
		fmt.Printf("❌ [Upload Error] Media upload failed: %v\n", err)
		return
	}

	// لسٹ مینیو بٹن
	listMsg := &waProto.ListMessage{
		Title:       proto.String("IMPOSSIBLE BOT"),
		Description: proto.String("Select a command below:"),
		ButtonText:  proto.String("MENU"),
		ListType:    waProto.ListMessage_SINGLE_SELECT.Enum(),
		Sections: []*waProto.ListMessage_Section{
			{
				Title: proto.String("TOOLS"),
				Rows: []*waProto.ListMessage_Row{
					{Title: proto.String("Ping Status"), RowID: proto.String("ping")},
					{Title: proto.String("Check ID"), RowID: proto.String("id")},
				},
			},
		},
	}

	// امیج میسج بنانا
	imageMsg := &waProto.ImageMessage{
		Mimetype:      proto.String("image/png"),
		Caption:       proto.String("*📜 IMPOSSIBLE MENU*\n\nHi! Use the button below to see commands."),
		Url:           &uploadResp.URL,
		DirectPath:    &uploadResp.DirectPath,
		MediaKey:      uploadResp.MediaKey,
		FileEncSha256: uploadResp.FileEncSha256,
		FileSha256:    uploadResp.FileSha256,
		FileLength:    proto.Uint64(uint64(len(imgData))),
	}

	// مکمل میسج جس میں تصویر اور مینیو دونوں ہوں
	msg := &waProto.Message{
		ImageMessage: imageMsg,
		ListMessage:  listMsg,
	}

	fmt.Println("📧 [Delivery] Sending full menu bundle...")
	resp, sendErr := client.SendMessage(context.Background(), chat, msg)
	
	if sendErr != nil {
		fmt.Printf("❌ [Send Error] Deliver failed: %v\n", sendErr)
	} else {
		fmt.Printf("✅ [Delivery] Sent! Message ID: %s\n", resp.ID)
	}
}

func handlePairAPI(c *gin.Context) {
	var req struct{ Number string `json:"number"` }
	c.BindJSON(&req)
	cleanNum := strings.ReplaceAll(req.Number, "+", "")
	
	fmt.Printf("🧹 [Security] Fresh pairing for: %s\n", cleanNum)

	devices, _ := container.GetAllDevices(context.Background())
	for _, dev := range devices {
		if dev.ID != nil && strings.Contains(dev.ID.User, cleanNum) {
			container.DeleteDevice(context.Background(), dev)
		}
	}

	newDevice := container.NewDevice()
	if client.IsConnected() { client.Disconnect() }
	client = whatsmeow.NewClient(newDevice, waLog.Stdout("Client", "INFO", true))
	client.AddEventHandler(eventHandler)
	client.Connect()
	
	time.Sleep(10 * time.Second)
	code, err := client.PairPhone(context.Background(), cleanNum, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"code": code})
}