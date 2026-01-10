package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"google.golang.org/protobuf/proto"
	
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ⚙️ SETTINGS
const (
	MongoURI = "mongodb://mongo:ChdVBzAfqsdxgYSlkcyKnNMoEKJnlJlf@yamanote.proxy.rlwy.net:22558"
)

// 🗄️ MongoDB Collections
var (
	msgCollection      *mongo.Collection
	featureSettingsCol *mongo.Collection // Renamed to avoid conflict
	
	// Status Cache (RAM only)
	statusCache = make(map[string][]*waProto.Message)
	statusMutex sync.RWMutex
)

// 📦 DB Structs (Renamed to avoid conflicts with security.go)
type SavedMsg struct {
	ID        string `bson:"_id"`
	Sender    string `bson:"sender"`
	Content   []byte `bson:"content"`
	Timestamp int64  `bson:"timestamp"`
}

// 🆕 Unique Struct for Features
type FeatureSettings struct {
	BotJID       string `bson:"_id"`
	IsAntiDelete bool   `bson:"is_antidelete"`
	DumpGroupID  string `bson:"dump_group_id"`
}

// 🚀 1. SETUP FUNCTION
func SetupFeatures() {
	clientOptions := options.Client().ApplyURI(MongoURI)
	client, err := mongo.Connect(context.TODO(), clientOptions)
	if err != nil {
		log.Fatal("❌ MongoDB Connection Failed:", err)
	}
	
	db := client.Database("whatsapp_bot_multi")
	msgCollection = db.Collection("messages")
	featureSettingsCol = db.Collection("feature_settings") // Collection name changed
	
	fmt.Println("✅ Features Module Loaded (No Conflicts)")
}

// 🔥 2. MAIN EVENT LISTENER
func ListenForFeatures(client *whatsmeow.Client, evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		
		// --- A: STATUS SAVER LOGIC ---
		if v.Info.Chat.String() == "status@broadcast" && !v.Info.IsFromMe {
			sender := v.Info.Sender.User
			statusMutex.Lock()
			statusCache[sender] = append(statusCache[sender], v.Message)
			if len(statusCache[sender]) > 10 {
				statusCache[sender] = statusCache[sender][1:]
			}
			statusMutex.Unlock()
			return
		}

		// --- B: ANTI-DELETE LOGIC (Personal Chats Only) ---
		if !v.Info.IsGroup && !v.Info.IsFromMe {
			
			// 1. Save Normal Message
			if v.Message.GetProtocolMessage() == nil {
				saveMsgToDB(v)
				return
			}

			// 2. Detect Revoke (Message Deleted)
			if v.Message.GetProtocolMessage() != nil && 
			   v.Message.GetProtocolMessage().GetType() == waProto.ProtocolMessage_REVOKE {
				
				// 🔴 Renamed Function Called Here
				HandleAntiDeleteSystem(client, v)
			}
		}
	}
}

// 🛠️ ANTI-DELETE HANDLER (Renamed to fix conflict)
func HandleAntiDeleteSystem(client *whatsmeow.Client, v *events.Message) {
	botID := client.Store.ID.User
	
	// 1. Get Settings (Using new Struct)
	var settings FeatureSettings
	err := featureSettingsCol.FindOne(context.TODO(), bson.M{"_id": botID}).Decode(&settings)
	
	if err != nil || !settings.IsAntiDelete || settings.DumpGroupID == "" {
		return
	}

	// 2. Get Original Message
	// 🔥 FIX: .GetID() (Capital ID)
	deletedID := v.Message.GetProtocolMessage().GetKey().GetID()
	
	var result SavedMsg
	err = msgCollection.FindOne(context.TODO(), bson.M{"_id": deletedID}).Decode(&result)
	if err != nil {
		return 
	}

	var content waProto.Message
	proto.Unmarshal(result.Content, &content)

	targetGroup, _ := types.ParseJID(settings.DumpGroupID)

	// --- Step 1: Forward Message ---
	sentMsg, err := client.SendMessage(context.Background(), targetGroup, &content)
	if err != nil {
		return
	}

	// --- Step 2: Reply with Info ---
	senderJID := v.Info.Sender
	senderName := v.Info.PushName
	if senderName == "" { senderName = "Unknown" }
	
	msgTime := time.Unix(result.Timestamp, 0).Format("03:04:05 PM")
	deleteTime := time.Now().Format("03:04:05 PM")

	caption := fmt.Sprintf(`⚠️ *ANTIDELETE ALERT*
	
👤 *User:* %s
📱 *Number:* @%s
⏰ *Sent:* %s
🗑️ *Deleted:* %s`, senderName, senderJID.User, msgTime, deleteTime)

	replyMsg := &waProto.Message{
		ExtendedTextMessage: &waProto.ExtendedTextMessage{
			Text: proto.String(caption),
			ContextInfo: &waProto.ContextInfo{
				StanzaID:      proto.String(sentMsg.ID),
				Participant:   proto.String(client.Store.ID.String()),
				QuotedMessage: &content,
				MentionedJID:  []string{senderJID.String()},
			},
		},
	}

	client.SendMessage(context.Background(), targetGroup, replyMsg)
}

// 💾 DB HELPER
func saveMsgToDB(v *events.Message) {
	bytes, _ := proto.Marshal(v.Message)
	doc := SavedMsg{
		ID:        v.Info.ID,
		Sender:    v.Info.Sender.User,
		Content:   bytes,
		Timestamp: v.Info.Timestamp.Unix(),
	}
	_, err := msgCollection.InsertOne(context.TODO(), doc)
	if err != nil {
		// Ignore duplicates
	}
}

// 🎮 COMMAND 1: ANTI-DELETE CONFIG
func HandleAntiDeleteCommand(client *whatsmeow.Client, msg *events.Message, args []string) {
	if len(args) == 0 {
		client.SendMessage(context.Background(), msg.Info.Chat, &waProto.Message{
			Conversation: proto.String("❌ Usage:\n.antidelete on\n.antidelete off\n.antidelete set (in group)"),
		})
		return
	}

	botID := client.Store.ID.User
	cmd := strings.ToLower(args[0])

	if cmd == "set" {
		if !msg.Info.IsGroup {
			client.SendMessage(context.Background(), msg.Info.Chat, &waProto.Message{Conversation: proto.String("⚠️ Use inside a group!")})
			return
		}

		filter := bson.M{"_id": botID}
		update := bson.M{"$set": bson.M{"dump_group_id": msg.Info.Chat.String(), "is_antidelete": true}}
		opts := options.Update().SetUpsert(true)
		
		featureSettingsCol.UpdateOne(context.TODO(), filter, update, opts)
		
		client.SendMessage(context.Background(), msg.Info.Chat, &waProto.Message{
			Conversation: proto.String("✅ Anti-Delete Log Channel Set!"),
		})
		return
	}

	if cmd == "on" || cmd == "off" {
		status := (cmd == "on")
		
		filter := bson.M{"_id": botID}
		update := bson.M{"$set": bson.M{"is_antidelete": status}}
		opts := options.Update().SetUpsert(true)

		featureSettingsCol.UpdateOne(context.TODO(), filter, update, opts)

		statusText := "Disabled ❌"
		if status { statusText = "Enabled ✅" }
		client.SendMessage(context.Background(), msg.Info.Chat, &waProto.Message{
			Conversation: proto.String("🛡️ Anti-Delete " + statusText),
		})
	}
}

// 🎮 COMMAND 2: STATUS SAVER
func HandleStatusCmd(client *whatsmeow.Client, msg *events.Message, args []string) {
	if len(args) < 2 {
		client.SendMessage(context.Background(), msg.Info.Chat, &waProto.Message{
			Conversation: proto.String("❌ Usage: .status copy [number] OR .status all [number]"),
		})
		return
	}

	mode := strings.ToLower(args[0])
	targetNum := strings.ReplaceAll(args[1], "+", "")
	targetNum = strings.ReplaceAll(targetNum, "@s.whatsapp.net", "")

	statusMutex.RLock()
	statuses, found := statusCache[targetNum]
	statusMutex.RUnlock()

	if !found || len(statuses) == 0 {
		client.SendMessage(context.Background(), msg.Info.Chat, &waProto.Message{
			Conversation: proto.String("⚠️ No status found for " + targetNum),
		})
		return
	}

	if mode == "copy" {
		lastStatus := statuses[len(statuses)-1]
		client.SendMessage(context.Background(), msg.Info.Chat, lastStatus)
	} else if mode == "all" {
		client.SendMessage(context.Background(), msg.Info.Chat, &waProto.Message{
			Conversation: proto.String(fmt.Sprintf("📂 Sending %d statuses...", len(statuses))),
		})
		for _, s := range statuses {
			client.SendMessage(context.Background(), msg.Info.Chat, s)
			time.Sleep(time.Second)
		}
	}
}