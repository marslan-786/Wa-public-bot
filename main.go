package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
	
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// --- 🌐 GLOBAL VARIABLES ---
var (
	container   *sqlstore.Container
	clientMap   = make(map[string]*whatsmeow.Client)
	clientMutex sync.RWMutex
	
	// MongoDB Client
	mongoClient *mongo.Client
	mongoColl   *mongo.Collection
	
	// WebSocket
	wsupgrader = websocket.Upgrader{
		ReadBufferSize:   1024,
		WriteBufferSize:  1024,
		HandshakeTimeout: 10 * time.Second,
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	clients = make(map[*websocket.Conn]bool)
	wsMutex sync.Mutex
)

// --- 🚀 MAIN START ---
func main() {
	fmt.Println("🚀 IMPOSSIBLE BOT FINAL V4 | STARTING SYSTEM...")

	// 1. Connect MongoDB (For Data Persistence)
	connectMongo()

	// 2. Load Global Data from Mongo (Defined in commands.go)
	loadDataFromMongo()

	// 3. Connect PostgreSQL (For WhatsApp Sessions)
	dbURL := os.Getenv("DATABASE_URL")
	dbType := "postgres"
	if dbURL == "" {
		dbType = "sqlite3"
		dbURL = "file:impossible_sessions.db?_foreign_keys=on"
		fmt.Println("⚠️ Using SQLite. Set DATABASE_URL in Railway for persistent sessions.")
	} else {
		fmt.Println("✅ Using PostgreSQL for Sessions.")
	}

	dbLog := waLog.Stdout("DB", "INFO", true)
	var err error
	container, err = sqlstore.New(context.Background(), dbType, dbURL, dbLog)
	if err != nil {
		log.Fatalf("❌ DB Error: %v", err)
	}

	// 4. Restore Sessions
	devices, err := container.GetAllDevices(context.Background())
	if err == nil {
		fmt.Printf("🔄 Restoring %d sessions...\n", len(devices))
		for _, device := range devices {
			go connectClient(device)
		}
	}

	// 5. Web Server
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	r.LoadHTMLGlob("web/*.html")
	r.StaticFile("/pic.png", "./pic.png") // Fallback if in root
	r.Static("/web", "./web")             // Serve web folder

	r.GET("/", func(c *gin.Context) { c.HTML(200, "index.html", nil) })
	r.GET("/ws", handleWebSocket)
	r.POST("/api/pair", handlePairing)

	go r.Run(":8080")
	fmt.Println("🌐 Server running on :8080")

	// 6. Shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	fmt.Println("🔻 Shutting down...")
	clientMutex.Lock()
	for _, cli := range clientMap {
		cli.Disconnect()
	}
	clientMutex.Unlock()
	
	if mongoClient != nil {
		mongoClient.Disconnect(context.Background())
	}
}

// --- 🍃 MONGODB CONNECTION ---
func connectMongo() {
	// User provided URL
	mongoURL := "mongodb://mongo:AEvrikOWlrmJCQrDTQgfGtqLlwhwLuAA@crossover.proxy.rlwy.net:29609"
	
	// Check if ENV overrides it
	if envUrl := os.Getenv("MONGO_URL"); envUrl != "" {
		mongoURL = envUrl
	}

	serverAPI := options.ServerAPI(options.ServerAPIVersion1)
	opts := options.Client().ApplyURI(mongoURL).SetServerAPIOptions(serverAPI)
	
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var err error
	mongoClient, err = mongo.Connect(ctx, opts)
	if err != nil {
		log.Fatal("❌ MongoDB Connection Error: ", err)
	}

	// Ping to verify
	if err := mongoClient.Ping(ctx, nil); err != nil {
		log.Fatal("❌ MongoDB Ping Failed: ", err)
	}

	fmt.Println("✅ Connected to MongoDB (Impossible Cluster)")
	mongoColl = mongoClient.Database("impossible_bot").Collection("settings")
}

// --- 🔌 CLIENT CONNECTION ---
func connectClient(device *store.Device) {
	client := whatsmeow.NewClient(device, waLog.Stdout("Client", "INFO", true))
	client.AddEventHandler(func(evt interface{}) { handler(client, evt) })

	if err := client.Connect(); err == nil && client.Store.ID != nil {
		clientMutex.Lock()
		clientMap[client.Store.ID.String()] = client
		clientMutex.Unlock()
		
		msg := fmt.Sprintf("✅ Connected: %s", client.Store.ID.User)
		fmt.Println(msg)
		broadcastWS(gin.H{"type": "log", "msg": msg})
		
		dataMutex.RLock()
		if data.AlwaysOnline {
			client.SendPresence(context.Background(), types.PresenceAvailable)
		}
		dataMutex.RUnlock()
	}
}

// --- 🔗 PAIRING ---
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

	client.AddEventHandler(func(evt interface{}) { handler(client, evt) })
	
	clientMutex.Lock()
	// Temporary map add, explicit add happens on Login event usually
	if client.Store.ID != nil {
		clientMap[client.Store.ID.String()] = client
	}
	clientMutex.Unlock()

	c.JSON(200, gin.H{"code": code})
}

// --- 📡 WEBSOCKET ---
func handleWebSocket(c *gin.Context) {
	conn, err := wsupgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil { return }
	
	wsMutex.Lock()
	clients[conn] = true
	wsMutex.Unlock()

	clientMutex.RLock()
	count := len(clientMap)
	clientMutex.RUnlock()
	conn.WriteJSON(gin.H{"type": "stats", "sessions": count, "uptime": time.Since(startTime).String()})

	defer func() {
		wsMutex.Lock()
		delete(clients, conn)
		wsMutex.Unlock()
		conn.Close()
	}()

	for {
		if _, _, err := conn.ReadMessage(); err != nil { break }
	}
}

func broadcastWS(msg interface{}) {
	wsMutex.Lock()
	defer wsMutex.Unlock()
	for client := range clients {
		client.WriteJSON(msg)
	}
}