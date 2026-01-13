package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types/events"
	waProto "go.mau.fi/whatsmeow/binary/proto"
)

// --- ⚙️ CONFIGURATION ---
// یوزر کی ڈیمانڈ کے مطابق 1GB کا ٹکڑا
const ChunkSize int64 = 1024 * 1024 * 1024 

// --- 🧠 MEMORY SYSTEM ---
type MovieResult struct {
	Identifier string
	Title      string
	Year       string
	Downloads  int
}

var searchCache = make(map[string][]MovieResult)
var movieMutex sync.Mutex 

// Archive API Response Structures
type IAHeader struct {
	Identifier string      `json:"identifier"`
	Title      string      `json:"title"`
	Year       interface{} `json:"year"`
	Downloads  interface{} `json:"downloads"`
}

type IAResponse struct {
	Response struct {
		Docs []IAHeader `json:"docs"`
	} `json:"response"`
}

type IAMetadata struct {
	Files []struct {
		Name   string `json:"name"`
		Format string `json:"format"`
		Size   string `json:"size"` 
	} `json:"files"`
}

// --- 🎮 MAIN HANDLER (No Changes here) ---
func handleArchive(client *whatsmeow.Client, v *events.Message, input string) {
	if input == "" { return }
	input = strings.TrimSpace(input)
	senderJID := v.Info.Sender.String()

	// --- 1️⃣ کیا یوزر نے نمبر سلیکٹ کیا ہے؟ ---
	if isNumber(input) {
		index, _ := strconv.Atoi(input)
		
		movieMutex.Lock()
		movies, exists := searchCache[senderJID]
		movieMutex.Unlock()

		if exists && index > 0 && index <= len(movies) {
			selectedMovie := movies[index-1]
			
			react(client, v.Info.Chat, v.Info.ID, "🔄")
			replyMessage(client, v, fmt.Sprintf("🔎 *Checking files for:* %s\nPlease wait...", selectedMovie.Title))
			
			go downloadFromIdentifier(client, v, selectedMovie)
			return
		}
	}

	// --- 2️⃣ کیا یہ ڈائریکٹ لنک ہے؟ ---
	if strings.HasPrefix(input, "http") {
		react(client, v.Info.Chat, v.Info.ID, "🔗")
		replyMessage(client, v, "⏳ *Processing Direct Link...*")
		// ڈائریکٹ لنک کے لیے بھی نیا اسٹریمر فنکشن یوز ہوگا
		go streamDownloadManager(client, v, input, "Unknown_File")
		return
	}

	// --- 3️⃣ یہ سرچ کوئری ہے! ---
	react(client, v.Info.Chat, v.Info.ID, "🔎")
	go performSearch(client, v, input, senderJID)
}

// --- 🔍 Search Engine (No Changes) ---
func performSearch(client *whatsmeow.Client, v *events.Message, query string, senderJID string) {
	encodedQuery := url.QueryEscape(fmt.Sprintf("title:(%s) AND mediatype:(movies)", query))
	apiURL := fmt.Sprintf("https://archive.org/advancedsearch.php?q=%s&fl[]=identifier&fl[]=title&fl[]=year&fl[]=downloads&sort[]=downloads+desc&output=json&rows=10", encodedQuery)

	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")

	clientHttp := &http.Client{Timeout: 30 * time.Second}
	resp, err := clientHttp.Do(req)
	
	if err != nil {
		replyMessage(client, v, "❌ Network Error: Could not reach Archive API.")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		replyMessage(client, v, fmt.Sprintf("❌ API Error: %d", resp.StatusCode))
		return
	}

	var result IAResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		replyMessage(client, v, "❌ Data Parse Error (Invalid JSON).")
		return
	}

	docs := result.Response.Docs
	if len(docs) == 0 {
		replyMessage(client, v, "🚫 No movies found. Please Check Your Spelling or Try a different name.")
		return
	}

	var movieList []MovieResult
	msgText := fmt.Sprintf("🎬 *Archive Results for:* '%s'\n\n", query)

	for i, doc := range docs {
		yearStr := fmt.Sprintf("%v", doc.Year)
		
		dlCount := 0
		switch val := doc.Downloads.(type) {
		case float64:
			dlCount = int(val)
		case string:
			dlCount, _ = strconv.Atoi(val)
		}

		movieList = append(movieList, MovieResult{
			Identifier: doc.Identifier,
			Title:      doc.Title,
			Year:       yearStr,
			Downloads:  dlCount,
		})
		msgText += fmt.Sprintf("*%d.* %s (%s)\n", i+1, doc.Title, yearStr)
	}
	
	msgText += "\n👇 *Reply with a number to download.*"

	movieMutex.Lock()
	searchCache[senderJID] = movieList
	movieMutex.Unlock()

	client.SendMessage(context.Background(), v.Info.Chat, &waProto.Message{
		ExtendedTextMessage: &waProto.ExtendedTextMessage{
			Text: proto.String(msgText),
			ContextInfo: &waProto.ContextInfo{
				StanzaID:      proto.String(v.Info.ID),
				Participant:   proto.String(v.Info.Sender.String()),
				QuotedMessage: v.Message,
			},
		},
	})
}

// --- 📥 Metadata Fetcher (Updated to call Streamer) ---
func downloadFromIdentifier(client *whatsmeow.Client, v *events.Message, movie MovieResult) {
	fmt.Println("🔍 [ARCHIVE] Fetching metadata for:", movie.Identifier)
	
	metaURL := fmt.Sprintf("https://archive.org/metadata/%s", movie.Identifier)
	req, _ := http.NewRequest("GET", metaURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	
	clientHttp := &http.Client{Timeout: 30 * time.Second}
	resp, err := clientHttp.Do(req)
	
	if err != nil { return }
	defer resp.Body.Close()

	var meta IAMetadata
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		replyMessage(client, v, "❌ Metadata Error: JSON parse failed.")
		return
	}

	bestFile := ""
	maxSize := int64(0)

	for _, f := range meta.Files {
		fName := strings.ToLower(f.Name)
		if strings.HasSuffix(fName, ".mp4") || strings.HasSuffix(fName, ".mkv") {
			s, _ := strconv.ParseInt(f.Size, 10, 64)
			if s > maxSize {
				maxSize = s
				bestFile = f.Name
			}
		}
	}

	if bestFile == "" {
		replyMessage(client, v, "❌ No suitable video file found.")
		return
	}

	finalURL := fmt.Sprintf("https://archive.org/download/%s/%s", movie.Identifier, url.PathEscape(bestFile))
	sizeMB := float64(maxSize) / (1024 * 1024)
	
	// 🔥 Warning logic simplified
	extraWarning := ""
	if sizeMB > 1000 { // 1000MB = 1GB
		extraWarning = "\n⚠️ *File > 1GB:* Sending in parts via Disk Stream."
	}

	infoMsg := fmt.Sprintf("🚀 *Starting Download!*\n\n🎬 *Title:* %s\n📊 *Size:* %.2f MB%s\n\n_Streaming via Disk Buffer..._", movie.Title, sizeMB, extraWarning)
	replyMessage(client, v, infoMsg)
	
	// 👇 پرانے فنکشن کی جگہ اب نیا اسٹریمر کال ہوگا
	streamDownloadManager(client, v, finalURL, movie.Title)
}

// --- 🚀 NEW: DISK-BASED PIPELINE MANAGER ---
// یہ فنکشن پرانے downloadFileDirectly اور splitAndSend کو ضم (Merge) کر کے بنایا گیا ہے
func streamDownloadManager(client *whatsmeow.Client, v *events.Message, urlStr string, customTitle string) {
	// 1. سرور سے کنکشن بنائیں
	req, _ := http.NewRequest("GET", urlStr, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	clientHttp := &http.Client{Timeout: 0} // Timeout ختم
	
	resp, err := clientHttp.Do(req)
	if err != nil {
		replyMessage(client, v, "❌ Connection Error.")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		replyMessage(client, v, "❌ Server Error: Could not access file.")
		return
	}

	// نام کی صفائی
	if customTitle == "Unknown_File" { 
		parts := strings.Split(urlStr, "/")
		customTitle = parts[len(parts)-1]
	}
	customTitle = strings.ReplaceAll(customTitle, "/", "_")
	if !strings.Contains(customTitle, ".") { customTitle += ".mp4" }

	partNum := 1
	copyBuffer := make([]byte, 32*1024) // 32KB buffer for IO operations

	for {
		// 2. ڈسک پر ٹیمپ فائل بنائیں (ریم استعمال نہیں ہوگی)
		partFileName := fmt.Sprintf("stream_buffer_%d_part_%d.mp4", time.Now().UnixNano(), partNum)
		fileOnDisk, err := os.Create(partFileName)
		if err != nil {
			replyMessage(client, v, "❌ Disk Error: Cannot create buffer file.")
			return
		}

		// 3. ✨ PIPING MAGIC: نیٹ ورک سے 1GB ڈیٹا سیدھا ڈسک فائل میں
		// io.LimitReader صرف 1GB اٹھائے گا اور رک جائے گا
		written, err := io.CopyBuffer(io.LimitReader(resp.Body, ChunkSize), fileOnDisk, copyBuffer)
		fileOnDisk.Close() // فائل محفوظ، اب بند

		if written > 0 {
			fmt.Printf("💾 Part %d Saved to Disk (%.2f MB). Uploading...\n", partNum, float64(written)/(1024*1024))
			
			// 4. ڈسک سے اپلوڈ کریں
			uploadErr := uploadChunkFromDisk(client, v, partFileName, customTitle, partNum)
			
			// 5. 🔥 اہم: فائل فوراً ڈیلیٹ کریں
			os.Remove(partFileName) 
			
			// ریم صفائی
			debug.FreeOSMemory()

			if uploadErr != nil {
				replyMessage(client, v, fmt.Sprintf("❌ Upload Failed for Part %d", partNum))
				return
			}
		}

		// اگر فائل ختم ہو گئی (EOF)
		if err == io.EOF {
			break
		}
		if err != nil {
			replyMessage(client, v, "❌ Stream Interrupted from Source.")
			break
		}

		partNum++
	}

	react(client, v.Info.Chat, v.Info.ID, "✅")
	replyMessage(client, v, "✅ *Completed!*")
}

// 📤 Helper: Upload Single Chunk
func uploadChunkFromDisk(client *whatsmeow.Client, v *events.Message, path string, originalName string, partNum int) error {
	// فائل ڈسک سے پڑھیں
	fileData, err := os.ReadFile(path)
	if err != nil { return err }

	// اپلوڈ کریں
	up, err := client.Upload(context.Background(), fileData, whatsmeow.MediaDocument)
	
	// میموری خالی کریں
	fileData = nil 
	runtime.GC() 

	if err != nil { return err }

	finalName := fmt.Sprintf("%s_Part_%d.mp4", originalName, partNum)
	caption := fmt.Sprintf("💿 *Part %d* \n📂 %s", partNum, originalName)

	// میسج سینڈ کریں
	return client.SendMessage(context.Background(), v.Info.Chat, &waProto.Message{
		DocumentMessage: &waProto.DocumentMessage{
			URL:           proto.String(up.URL),
			DirectPath:    proto.String(up.DirectPath),
			MediaKey:      up.MediaKey,
			Mimetype:      proto.String("video/mp4"),
			Title:         proto.String(finalName),
			FileName:      proto.String(finalName),
			FileLength:    proto.Uint64(uint64(up.FileLength)),
			FileSHA256:    up.FileSHA256,
			FileEncSHA256: up.FileEncSHA256,
			Caption:       proto.String(caption),
		},
	}).Error
}

// --- 🛠️ UTILS ---
func isNumber(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}