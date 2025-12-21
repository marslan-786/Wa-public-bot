package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
	"github.com/redis/go-redis/v9" // ✅ ریڈیس لائبریری
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
	rdb      *redis.Client     // ✅ یہ مسنگ تھا، اسی لیے 'undefined' آ رہا ہے
    ctx      = context.Background() // ✅ ریڈیس کے لیے کانٹیکسٹ
)

var (
	client           *whatsmeow.Client
	container        *sqlstore.Container
	rdb              *redis.Client // ✅ ریڈیس کلائنٹ
	ctx              = context.Background()
	upgrader         = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	wsClients = make(map[*websocket.Conn]bool)

	// ⚡ الٹرا فاسٹ کیشنگ
	botCleanIDCache = make(map[string]string)
	botPrefixes     = make(map[string]string)
	prefixMutex     sync.RWMutex
	clientsMutex    sync.RWMutex
	activeClients   = make(map[string]*whatsmeow.Client)
)

// ✅ 1. ریڈیس کنکشن (سائنس دانوں کو حیران کرنے کے لئے)
func initRedis() {
	redisURL := os.Getenv("REDIS_URL") // ریلوے کا ویری ایبل
	if redisURL == "" {
		redisURL = "redis://localhost:6379" // لوکل بیک اپ
	}

	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Fatalf("❌ Redis URL parsing failed: %v", err)
	}

	rdb = redis.NewClient(opt)

	// چیک کریں کہ کیا ریڈیس آن لائن ہے
	_, err = rdb.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("❌ Redis connection failed: %v", err)
	}
	fmt.Println("🚀 [REDIS] Connected Successfully! Zero Latency Mode Active.")
}

func main() {
	fmt.Println("🚀 IMPOSSIBLE BOT | START")

	// 1. ریڈیس اور اپ ٹائم کی شروعات
	initRedis()
	loadPersistentUptime()
	startPersistentUptimeTracker()

	// 2. واٹس ایپ ڈیٹا بیس (SQLite/Postgres)
	dbURL := os.Getenv("DATABASE_URL")
	dbType := "postgres"
	if dbURL == "" {
		dbType = "sqlite3"
		dbURL = "file:impossible.db?_foreign_keys=on"
	}

	dbLog := waLog.Stdout("Database", "ERROR", true)
	var err error
	container, err = sqlstore.New(context.Background(), dbType, dbURL, dbLog)
	if err != nil {
		log.Fatalf("❌ DB error: %v", err)
	}
	dbContainer = container

	// 3. ملٹی بوٹ سسٹم شروع کریں
	fmt.Println("🤖 Initializing Multi-Bot System...")
	StartAllBots(container)

	// 4. باقی سسٹمز
	InitLIDSystem()
	// لوڈ پریفکس فروم ریڈیس (ہم مونگو کو مکمل بائی پاس کر رہے ہیں)

	// 5. ویب سرور روٹس
	http.HandleFunc("/", serveHTML)
	http.HandleFunc("/pic.png", servePicture)
	http.HandleFunc("/ws", handleWebSocket)
	http.HandleFunc("/api/pair", handlePairAPI)
	http.HandleFunc("/link/pair/", handlePairAPILegacy)
	http.HandleFunc("/link/delete", handleDeleteSession)
	http.HandleFunc("/del/all", handleDelAllAPI)
	http.HandleFunc("/del/", handleDelNumberAPI)

	port := os.Getenv("PORT")
	if port == "" { port = "8080" }

	go func() {
		fmt.Printf("🌐 Web Server running on port %s\n", port)
		if err := http.ListenAndServe(":"+port, nil); err != nil {
			log.Printf("❌ Server error: %v\n", err)
		}
	}()

	// 6. شٹ ڈاؤن ہینڈلنگ
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	fmt.Println("\n🛑 Shutting down system...")
	clientsMutex.Lock()
	for id, activeClient := range activeClients {
		fmt.Printf("🔌 Disconnecting Bot: %s\n", id)
		activeClient.Disconnect()
	}
	clientsMutex.Unlock()
	fmt.Println("👋 Goodbye!")
}

// ✅ ⚡ بوٹ کنیکٹ ہوتے ہی آئی ڈی اور پریفکس کیش کریں
func ConnectNewSession(device *store.Device) {
	rawID := device.ID.User
	cleanID := getCleanID(rawID)
	
	// 1. آئی ڈی کیش میں ڈالیں
	clientsMutex.Lock()
	botCleanIDCache[rawID] = cleanID
	clientsMutex.Unlock()

	// 2. ریڈیس سے پریفکس اٹھائیں
	p, err := rdb.Get(ctx, "prefix:"+cleanID).Result()
	if err != nil { p = "." } // اگر نہیں ہے تو ڈیفالٹ
	
	prefixMutex.Lock()
	botPrefixes[cleanID] = p
	prefixMutex.Unlock()

	// ڈپلیکیٹ چیک
	clientsMutex.RLock()
	_, exists := activeClients[cleanID]
	clientsMutex.RUnlock()
	if exists { return }

	client := whatsmeow.NewClient(device, waLog.Stdout("Client", "ERROR", true))
	client.AddEventHandler(func(evt interface{}) { handler(client, evt) })

	if err := client.Connect(); err != nil {
		fmt.Printf("❌ [CONNECT ERROR] %s: %v\n", cleanID, err)
		return
	}

	clientsMutex.Lock()
	activeClients[cleanID] = client
	clientsMutex.Unlock()

	fmt.Printf("✅ [CONNECTED] Bot: %s | Prefix: %s\n", cleanID, p)
}

// ✅ ⚡ ریڈیس پریفکس اپڈیٹ (مونگو ڈی بی ریپلیسمنٹ)
func updatePrefixDB(botID string, newPrefix string) {
	prefixMutex.Lock()
	botPrefixes[botID] = newPrefix
	prefixMutex.Unlock()

	// ریڈیس میں سیو کریں (کبھی ڈیٹا ضائع نہیں ہوگا)
	err := rdb.Set(ctx, "prefix:"+botID, newPrefix, 0).Err()
	if err != nil {
		fmt.Printf("❌ [REDIS ERR] Could not save prefix: %v\n", err)
	}
}

// ... (باقی ویب روٹس اور ہینڈلرز ویسے ہی رہیں گے)


func serveHTML(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "web/index.html")
}

func servePicture(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "pic.png")
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	wsClients[conn] = true
	defer delete(wsClients, conn)

	status := map[string]interface{}{
		"connected": client != nil && client.IsConnected(),
		"session":   client != nil && client.Store.ID != nil,
	}
	conn.WriteJSON(status)

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

func broadcastWS(data interface{}) {
	for conn := range wsClients {
		conn.WriteJSON(data)
	}
}

// 1. تمام سیشنز ڈیلیٹ کرنے کی اے پی آئی
func handleDelAllAPI(w http.ResponseWriter, r *http.Request) {
	fmt.Println("🗑️ [API] Deleting all sessions...")
	
	// میموری سے کلائنٹس ڈس کنیکٹ کریں
	clientsMutex.Lock()
	for id, c := range activeClients {
		fmt.Printf("🔌 Disconnecting: %s\n", id)
		c.Disconnect()
		delete(activeClients, id)
	}
	clientsMutex.Unlock()

	// ڈیٹا بیس سے تمام ڈیوائسز اڑائیں
	devices, _ := container.GetAllDevices(context.Background())
	for _, dev := range devices {
		dev.Delete(context.Background())
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"success":true, "message":"All sessions wiped from DB and memory"}`)
}

// 2. مخصوص نمبر کا سیشن ڈیلیٹ کرنے کی اے پی آئی (/del/92301...)
func handleDelNumberAPI(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 {
		http.Error(w, `{"error":"Number required"}`, 400)
		return
	}
	targetNum := parts[2]
	fmt.Printf("🗑️ [API] Deleting session for: %s\n", targetNum)

	// میموری سے نکالیں
	clientsMutex.Lock()
	if c, ok := activeClients[getCleanID(targetNum)]; ok {
		c.Disconnect()
		delete(activeClients, getCleanID(targetNum))
	}
	clientsMutex.Unlock()

	// ڈیٹا بیس سے نکالیں
	devices, _ := container.GetAllDevices(context.Background())
	deleted := false
	for _, dev := range devices {
		if getCleanID(dev.ID.User) == getCleanID(targetNum) {
			dev.Delete(context.Background())
			deleted = true
			break
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if deleted {
		fmt.Fprintf(w, `{"success":true, "message":"Session deleted for %s"}`, targetNum)
	} else {
		fmt.Fprintf(w, `{"success":false, "message":"No session found for %s"}`, targetNum)
	}
}


func handlePairAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, `{"error":"Method not allowed"}`, 405)
		return
	}

	var req struct {
		Number string `json:"number"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid JSON"}`, 400)
		return
	}

	// نمبر کلین کریں
	number := strings.TrimSpace(req.Number)
	number = strings.ReplaceAll(number, "+", "")
	number = strings.ReplaceAll(number, " ", "")
	number = strings.ReplaceAll(number, "-", "")
	cleanNum := getCleanID(number)

	fmt.Printf("📱 [PAIRING] New request for: %s\n", cleanNum)

	// ✅ اہم سٹیپ: پہلے سے موجود سیشن چیک کریں اور ڈیلیٹ کریں
	devices, _ := container.GetAllDevices(context.Background())
	for _, dev := range devices {
		if getCleanID(dev.ID.User) == cleanNum {
			fmt.Printf("🧹 [CLEANUP] Removing old session for %s before re-pairing...\n", cleanNum)
			
			// میموری سے ہٹائیں
			clientsMutex.Lock()
			if c, ok := activeClients[cleanNum]; ok {
				c.Disconnect()
				delete(activeClients, cleanNum)
			}
			clientsMutex.Unlock()
			
			// ڈیٹا بیس سے ہٹائیں
			dev.Delete(context.Background())
		}
	}

	// اب نیا ڈیوائس اور پیرنگ کوڈ بنائیں
	newDevice := container.NewDevice()
	tempClient := whatsmeow.NewClient(newDevice, waLog.Stdout("Pairing", "INFO", true))
	
	tempClient.AddEventHandler(func(evt interface{}) {
        handler(tempClient, evt)
    })

	err := tempClient.Connect()
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), 500)
		return
	}

	// تھوڑا انتظار کریں تاکہ کنکشن مستحکم ہو
	time.Sleep(5 * time.Second)

	code, err := tempClient.PairPhone(context.Background(), number, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
	if err != nil {
		tempClient.Disconnect()
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), 500)
		return
	}

	fmt.Printf("✅ [CODE] Generated for %s: %s\n", cleanNum, code)

	broadcastWS(map[string]interface{}{
		"event": "pairing_code",
		"code":  code,
	})

	go func() {
		for i := 0; i < 60; i++ {
			time.Sleep(1 * time.Second)
			if tempClient.Store.ID != nil {
				fmt.Printf("🎉 [PAIRED] %s is now active!\n", cleanNum)
				clientsMutex.Lock()
				activeClients[cleanNum] = tempClient
				clientsMutex.Unlock()
				return
			}
		}
		tempClient.Disconnect()
	}()

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"success":true,"code":"%s"}`, code)
}


func handlePairAPILegacy(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		http.Error(w, `{"error":"Invalid URL"}`, 400)
		return
	}

	number := strings.TrimSpace(parts[3])
	number = strings.ReplaceAll(number, "+", "")
	number = strings.ReplaceAll(number, " ", "")
	number = strings.ReplaceAll(number, "-", "")

	if len(number) < 10 {
		http.Error(w, `{"error":"Invalid number"}`, 400)
		return
	}

	fmt.Printf("📱 Pairing: %s\n", number)

	if client != nil && client.IsConnected() {
		client.Disconnect()
		time.Sleep(10 * time.Second)
	}

	newDevice := container.NewDevice()
	tempClient := whatsmeow.NewClient(newDevice, waLog.Stdout("Pairing", "INFO", true))
	
	SetGlobalClient(tempClient)
	tempClient.AddEventHandler(func(evt interface{}) {
        handler(tempClient, evt)
    })

	err := tempClient.Connect()
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), 500)
		return
	}

	time.Sleep(10 * time.Second)

	code, err := tempClient.PairPhone(
		context.Background(),
		number,
		true,
		whatsmeow.PairClientChrome,
		"Chrome (Linux)",
	)

	if err != nil {
		tempClient.Disconnect()
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), 500)
		return
	}

	fmt.Printf("✅ Code: %s\n", code)

	go func() {
		for i := 0; i < 60; i++ {
			time.Sleep(1 * time.Second)
			if tempClient.Store.ID != nil {
				fmt.Println("✅ Paired!")
				client = tempClient
				
				OnNewPairing(client)
				
				return
			}
		}
		tempClient.Disconnect()
	}()

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"success":true,"code":"%s"}`, code)
}

func handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	if client != nil && client.IsConnected() {
		client.Disconnect()
	}

	devices, _ := container.GetAllDevices(context.Background())
	for _, device := range devices {
		device.Delete(context.Background())
	}

	broadcastWS(map[string]interface{}{
		"event":     "session_deleted",
		"connected": false,
	})

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"success":true,"message":"Session deleted"}`)
}