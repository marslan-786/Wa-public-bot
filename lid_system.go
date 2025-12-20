package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ════════════════════════════════════════════════════════════════
// 📁 LID DATA STRUCTURES
// ════════════════════════════════════════════════════════════════

type BotLIDInfo struct {
	Phone       string    `json:"phone" bson:"phone"`
	LID         string    `json:"lid" bson:"lid"`
	Platform    string    `json:"platform" bson:"platform"`
	SessionID   string    `json:"sessionId" bson:"sessionId"`
	ExtractedAt time.Time `json:"extractedAt" bson:"extractedAt"`
	LastUpdated time.Time `json:"lastUpdated" bson:"lastUpdated"`
}

type LIDDatabase struct {
	Timestamp time.Time              `json:"timestamp"`
	Count     int                    `json:"count"`
	Bots      map[string]BotLIDInfo `json:"bots"`
}

var (
	lidCache      = make(map[string]string) // phone -> lid
	lidCacheMutex sync.RWMutex
	lidDataFile   = "./lid_data.json"
	lidLogFile    = "./lid_extractor.log"
)

// ════════════════════════════════════════════════════════════════
// 🔧 HELPER FUNCTIONS
// ════════════════════════════════════════════════════════════════

// Extract clean number from JID
func getCleanNumber(jidStr string) string {
	if jidStr == "" {
		return ""
	}
	parts := strings.Split(jidStr, "@")
	userPart := parts[0]
	if strings.Contains(userPart, ":") {
		userPart = strings.Split(userPart, ":")[0]
	}
	return strings.TrimSpace(userPart)
}

// Get bot's phone number
func getBotPhoneNumber(client *whatsmeow.Client) string {
	if client.Store.ID == nil || client.Store.ID.IsEmpty() {
		return ""
	}
	return getCleanNumber(client.Store.ID.User)
}

// Get sender's phone number
func getSenderPhoneNumber(sender types.JID) string {
	if sender.IsEmpty() {
		return ""
	}
	return getCleanNumber(sender.User)
}

// ════════════════════════════════════════════════════════════════
// 🚀 NODE.JS LID EXTRACTOR RUNNER
// ════════════════════════════════════════════════════════════════

// Run Node.js LID extractor as child process
func runLIDExtractor() error {
	fmt.Println("\n╔═══════════════════════════════════════╗")
	fmt.Println("║   🔍 RUNNING LID EXTRACTOR           ║")
	fmt.Println("╚═══════════════════════════════════════╝\n")

	// Check if Node.js is available
	_, err := exec.LookPath("node")
	if err != nil {
		fmt.Println("⚠️ Node.js not found - skipping LID extraction")
		return fmt.Errorf("node.js not available")
	}

	// Check if lid-extractor.js exists
	extractorPath := "./lid-extractor.js"
	if _, err := os.Stat(extractorPath); os.IsNotExist(err) {
		fmt.Println("⚠️ lid-extractor.js not found - skipping")
		return fmt.Errorf("extractor script not found")
	}

	// Run Node.js script
	cmd := exec.Command("node", extractorPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Println("⏳ Extracting LIDs from sessions...")
	startTime := time.Now()

	if err := cmd.Run(); err != nil {
		fmt.Printf("⚠️ Extractor finished with warnings: %v\n", err)
		// Don't return error - might be normal on first run
	}

	duration := time.Since(startTime).Seconds()
	fmt.Printf("✅ Extraction completed in %.2fs\n\n", duration)

	return nil
}

// ════════════════════════════════════════════════════════════════
// 💾 LOAD LID DATA
// ════════════════════════════════════════════════════════════════

// Load LID data from JSON file
func loadLIDData() error {
	lidCacheMutex.Lock()
	defer lidCacheMutex.Unlock()

	// Check if file exists
	if _, err := os.Stat(lidDataFile); os.IsNotExist(err) {
		fmt.Println("⚠️ No LID data file found (normal on first run)")
		return nil
	}

	// Read file
	data, err := os.ReadFile(lidDataFile)
	if err != nil {
		return fmt.Errorf("failed to read LID data: %v", err)
	}

	// Parse JSON
	var lidDB LIDDatabase
	if err := json.Unmarshal(data, &lidDB); err != nil {
		return fmt.Errorf("failed to parse LID data: %v", err)
	}

	// Load into cache
	lidCache = make(map[string]string)
	for phone, botInfo := range lidDB.Bots {
		lidCache[phone] = botInfo.LID
	}

	fmt.Printf("✅ Loaded %d LID(s) from cache\n", len(lidCache))

	// Display loaded LIDs
	if len(lidCache) > 0 {
		fmt.Println("\n📊 Registered Bot LIDs:")
		for phone, lid := range lidCache {
			fmt.Printf("   📱 %s → 🆔 %s\n", phone, lid)
		}
		fmt.Println()
	}

	return nil
}

// ════════════════════════════════════════════════════════════════
// 💾 MONGODB INTEGRATION
// ════════════════════════════════════════════════════════════════

// Save LID to MongoDB
func saveLIDToMongo(botInfo BotLIDInfo) error {
	if mongoColl == nil {
		return fmt.Errorf("mongodb not connected")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Update or insert
	filter := bson.M{"phone": botInfo.Phone}
	update := bson.M{
		"$set": bson.M{
			"phone":       botInfo.Phone,
			"lid":         botInfo.LID,
			"platform":    botInfo.Platform,
			"sessionId":   botInfo.SessionID,
			"extractedAt": botInfo.ExtractedAt,
			"lastUpdated": time.Now(),
		},
	}

	opts := options.Update().SetUpsert(true)
	_, err := mongoColl.UpdateOne(ctx, filter, update, opts)
	
	if err != nil {
		return fmt.Errorf("mongodb save failed: %v", err)
	}

	fmt.Printf("✅ Saved to MongoDB: %s → %s\n", botInfo.Phone, botInfo.LID)
	return nil
}

// Load all LIDs from MongoDB
func loadLIDsFromMongo() error {
	if mongoColl == nil {
		return fmt.Errorf("mongodb not connected")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cursor, err := mongoColl.Find(ctx, bson.M{})
	if err != nil {
		return fmt.Errorf("mongodb query failed: %v", err)
	}
	defer cursor.Close(ctx)

	lidCacheMutex.Lock()
	defer lidCacheMutex.Unlock()

	count := 0
	for cursor.Next(ctx) {
		var botInfo BotLIDInfo
		if err := cursor.Decode(&botInfo); err != nil {
			continue
		}
		lidCache[botInfo.Phone] = botInfo.LID
		count++
	}

	if count > 0 {
		fmt.Printf("✅ Loaded %d LID(s) from MongoDB\n", count)
	}

	return nil
}

// Sync LID data to MongoDB
func syncLIDsToMongo() error {
	// Load from JSON first
	data, err := os.ReadFile(lidDataFile)
	if err != nil {
		return nil // No file to sync
	}

	var lidDB LIDDatabase
	if err := json.Unmarshal(data, &lidDB); err != nil {
		return err
	}

	// Save each to MongoDB
	for _, botInfo := range lidDB.Bots {
		if err := saveLIDToMongo(botInfo); err != nil {
			fmt.Printf("⚠️ Failed to sync %s: %v\n", botInfo.Phone, err)
		}
	}

	return nil
}

// ════════════════════════════════════════════════════════════════
// 🔐 OWNER VERIFICATION
// ════════════════════════════════════════════════════════════════

// Get LID for a phone number
func getLIDForPhone(phone string) string {
	lidCacheMutex.RLock()
	defer lidCacheMutex.RUnlock()

	cleanPhone := getCleanNumber(phone)
	if lid, exists := lidCache[cleanPhone]; exists {
		return lid
	}
	return ""
}

// Check if sender is owner using LID
func isOwnerByLID(client *whatsmeow.Client, sender types.JID) bool {
	botPhone := getBotPhoneNumber(client)
	if botPhone == "" {
		fmt.Println("⚠️ Cannot determine bot phone number")
		return false
	}

	// Get bot's LID from cache
	botLID := getLIDForPhone(botPhone)
	if botLID == "" {
		fmt.Printf("⚠️ No LID found for bot: %s\n", botPhone)
		return false
	}

	// Get sender's phone number
	senderPhone := getSenderPhoneNumber(sender)
	if senderPhone == "" {
		return false
	}

	// Compare: sender's phone should match bot's LID
	isMatch := (senderPhone == botLID)

	fmt.Printf("\n🔐 OWNER VERIFICATION\n")
	fmt.Printf("═══════════════════════════\n")
	fmt.Printf("📱 Bot Phone: %s\n", botPhone)
	fmt.Printf("🆔 Bot LID: %s\n", botLID)
	fmt.Printf("👤 Sender: %s\n", senderPhone)
	fmt.Printf("✅ Match: %v\n", isMatch)
	fmt.Printf("═══════════════════════════\n\n")

	return isMatch
}

// ════════════════════════════════════════════════════════════════
// 📊 COMMAND: OWNER VERIFICATION
// ════════════════════════════════════════════════════════════════

func sendOwnerStatus(client *whatsmeow.Client, v *events.Message) {
	botPhone := getBotPhoneNumber(client)
	botLID := getLIDForPhone(botPhone)
	senderPhone := getSenderPhoneNumber(v.Info.Sender)
	isOwn := isOwnerByLID(client, v.Info.Sender)

	status := "❌ NOT Owner"
	icon := "🚫"
	if isOwn {
		status = "✅ YOU are Owner"
		icon = "👑"
	}

	msg := fmt.Sprintf(`╔════════════════════╗
║ %s OWNER STATUS
╠════════════════════╣
║ 📱 Bot: %s
║ 🆔 LID: %s
║ 👤 You: %s
║ 
║ %s
╠════════════════════╣
║ 🔐 LID-Based Verification
╚════════════════════╝`,
		icon, botPhone, botLID, senderPhone, status)

	sendReplyMessage(client, v, msg)
}

// ════════════════════════════════════════════════════════════════
// 📊 COMMAND: LIST REGISTERED BOTS
// ════════════════════════════════════════════════════════════════

func sendBotsList(client *whatsmeow.Client, v *events.Message) {
	if !isOwnerByLID(client, v.Info.Sender) {
		replyMessage(client, v, "❌ Owner only command")
		return
	}

	lidCacheMutex.RLock()
	defer lidCacheMutex.RUnlock()

	if len(lidCache) == 0 {
		sendReplyMessage(client, v, "⚠️ No bots registered yet")
		return
	}

	msg := "╔════════════════════════════╗\n"
	msg += "║ 📊 REGISTERED BOTS\n"
	msg += "╠════════════════════════════╣\n"

	index := 1
	for phone, lid := range lidCache {
		msg += fmt.Sprintf("║ %d. %s\n", index, phone)
		msg += fmt.Sprintf("║    🆔 %s\n", lid)
		msg += "║\n"
		index++
	}

	msg += "╚════════════════════════════╝"
	sendReplyMessage(client, v, msg)
}

// ════════════════════════════════════════════════════════════════
// 🚀 INITIALIZATION SYSTEM
// ════════════════════════════════════════════════════════════════

// Initialize LID system (call this in main())
func InitLIDSystem() {
	fmt.Println("\n╔═══════════════════════════════════════╗")
	fmt.Println("║   🔐 LID SYSTEM INITIALIZING         ║")
	fmt.Println("╚═══════════════════════════════════════╝\n")

	// Step 1: Try to load from MongoDB first
	fmt.Println("📊 Checking MongoDB for existing LIDs...")
	if err := loadLIDsFromMongo(); err != nil {
		fmt.Printf("⚠️ MongoDB load failed: %v\n", err)
	}

	// Step 2: Run Node.js extractor
	fmt.Println("🔍 Running LID extractor...")
	if err := runLIDExtractor(); err != nil {
		fmt.Printf("⚠️ Extractor error: %v\n", err)
	}

	// Step 3: Load extracted data from JSON
	fmt.Println("📂 Loading LID data from file...")
	if err := loadLIDData(); err != nil {
		fmt.Printf("⚠️ Load error: %v\n", err)
	}

	// Step 4: Sync to MongoDB
	if mongoColl != nil {
		fmt.Println("💾 Syncing to MongoDB...")
		if err := syncLIDsToMongo(); err != nil {
			fmt.Printf("⚠️ Sync error: %v\n", err)
		}
	}

	// Final status
	lidCacheMutex.RLock()
	count := len(lidCache)
	lidCacheMutex.RUnlock()

	fmt.Println("\n╔═══════════════════════════════════════╗")
	if count > 0 {
		fmt.Printf("║   ✅ LID SYSTEM READY (%d bots)      ║\n", count)
	} else {
		fmt.Println("║   ⚠️ NO LIDS FOUND (First run?)      ║")
	}
	fmt.Println("╚═══════════════════════════════════════╝\n")

	// Display instructions if no LIDs
	if count == 0 {
		fmt.Println("📝 LID will be extracted on next bot pairing")
		fmt.Println("   Use /api/pair endpoint to pair new device")
		fmt.Println()
	}
}

// ════════════════════════════════════════════════════════════════
// 🔄 AUTO RE-EXTRACTION ON NEW PAIRING
// ════════════════════════════════════════════════════════════════

// Call this after successful pairing
func OnNewPairing(client *whatsmeow.Client) {
	fmt.Println("\n🔄 New pairing detected - extracting LID...")
	
	// Wait a bit for session to stabilize
	time.Sleep(3 * time.Second)
	
	// Run extractor again
	if err := runLIDExtractor(); err != nil {
		fmt.Printf("⚠️ Re-extraction failed: %v\n", err)
		return
	}
	
	// Reload data
	if err := loadLIDData(); err != nil {
		fmt.Printf("⚠️ Reload failed: %v\n", err)
		return
	}
	
	// Sync to MongoDB
	if mongoColl != nil {
		syncLIDsToMongo()
	}
	
	botPhone := getBotPhoneNumber(client)
	botLID := getLIDForPhone(botPhone)
	
	if botLID != "" {
		fmt.Printf("✅ New LID registered: %s → %s\n\n", botPhone, botLID)
	}
}

// ════════════════════════════════════════════════════════════════
// 🔧 PERMISSION SYSTEM
// ════════════════════════════════════════════════════════════════

// Updated permission check using LID
func canExecuteCommand(client *whatsmeow.Client, v *events.Message, cmd string) bool {
	// Owner always has access
	if isOwnerByLID(client, v.Info.Sender) {
		return true
	}

	// Private chats - allow all
	if !v.Info.IsGroup {
		return true
	}

	// Group mode checks
	s := getGroupSettings(v.Info.Chat.String())

	if s.Mode == "private" {
		return false
	}

	if s.Mode == "admin" {
		return isGroupAdmin(client, v.Info.Chat, v.Info.Sender)
	}

	return true // public mode
}

// Check if user is group admin
func isGroupAdmin(client *whatsmeow.Client, chat, user types.JID) bool {
	info, err := client.GetGroupInfo(context.Background(), chat)
	if err != nil {
		return false
	}

	userPhone := getSenderPhoneNumber(user)

	for _, p := range info.Participants {
		participantPhone := getSenderPhoneNumber(p.JID)
		if participantPhone == userPhone && (p.IsAdmin || p.IsSuperAdmin) {
			return true
		}
	}

	return false
}