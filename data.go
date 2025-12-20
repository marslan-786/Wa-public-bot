package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// --- 📡 LOAD DATA FROM MONGODB ---
func loadDataFromMongo() {
	if mongoColl == nil {
		log.Println("⚠️ MongoDB not initialized")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dataMutex.Lock()
	defer dataMutex.Unlock()

	// Load bot data
	var result BotData
	err := mongoColl.FindOne(ctx, bson.M{"_id": "bot_config"}).Decode(&result)
	
	if err != nil {
		// Create default data if not exists
		data = BotData{
			ID:            "bot_config",
			Prefix:        ".",
			AlwaysOnline:  false,
			AutoRead:      false,
			AutoReact:     false,
			AutoStatus:    false,
			StatusReact:   false,
			StatusTargets: []string{},
		}
		
		// Save default to MongoDB
		mongoColl.InsertOne(ctx, data)
		fmt.Println("✅ Default bot data created in MongoDB")
	} else {
		data = result
		fmt.Println("✅ Bot data loaded from MongoDB")
	}
}

// --- 💾 SAVE DATA TO MONGODB ---
func saveDataToMongo() {
	if mongoColl == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dataMutex.RLock()
	defer dataMutex.RUnlock()

	opts := options.Replace().SetUpsert(true)
	_, err := mongoColl.ReplaceOne(ctx, bson.M{"_id": "bot_config"}, data, opts)
	
	if err != nil {
		log.Printf("⚠️ Failed to save data: %v", err)
	}
}

// --- 📊 AUTO-SAVE ROUTINE ---
func startAutoSave() {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		
		for range ticker.C {
			saveDataToMongo()
		}
	}()
}