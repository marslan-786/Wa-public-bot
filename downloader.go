package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types/events"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"google.golang.org/protobuf/proto"
)

// ==================== ڈاؤن لوڈر سسٹم ====================

func handleTikTok(client *whatsmeow.Client, v *events.Message, url string) {
	if url == "" {
		msg := `╔═══════════════╗
║ 📝 TIKTOK
╠═══════════════
║ Usage:
║ .tiktok <url>
║
║ Example:
║ .tiktok https://
║ vm.tiktok.com/xx
╚═══════════════`
		replyMessage(client, v, msg)
		return
	}

	react(client, v.Info.Chat, v.Info.ID, "🎵")
	
	msg := `╔═══════════════╗
║ 🎵 PROCESSING
╠═══════════════
║ ⏳ Downloading
║ Please wait...
╚═══════════════`
	replyMessage(client, v, msg)

	type R struct {
		Data struct {
			Play string `json:"play"`
		} `json:"data"`
	}
	var r R
	err := getJson("https://www.tikwm.com/api/?url="+url, &r)
	
	if err == nil && r.Data.Play != "" {
		sendVideo(client, v, r.Data.Play, "🎵 *TikTok Video*\n✅ Successfully Downloaded")
	} else {
		replyMessage(client, v, "╔═══════════════╗\n║ ❌ FAILED\n╠═══════════════\n║ Download failed.\n║ Check URL.\n╚═══════════════")
	}
}

func handleFacebook(client *whatsmeow.Client, v *events.Message, url string) {
	if url == "" {
		msg := `╔═══════════════╗
║ 📘 FACEBOOK
╠═══════════════
║ Usage:
║ .fb <url>
║
║ Example:
║ .fb https://
║ fb.watch/xxxx
╚═══════════════`
		replyMessage(client, v, msg)
		return
	}

	react(client, v.Info.Chat, v.Info.ID, "📘")
	
	msg := `╔═══════════════╗
║ 📘 PROCESSING
╠═══════════════
║ ⏳ Downloading
║ Please wait...
╚═══════════════`
	replyMessage(client, v, msg)

	type R struct {
		BK9 struct {
			HD string `json:"HD"`
		} `json:"BK9"`
		Status bool `json:"status"`
	}
	var r R
	err := getJson("https://bk9.fun/downloader/facebook?url="+url, &r)
	
	if err == nil && r.BK9.HD != "" {
		sendVideo(client, v, r.BK9.HD, "📘 *Facebook Video*\n✅ Successfully Downloaded")
	} else {
		replyMessage(client, v, "╔═══════════════╗\n║ ❌ FAILED\n╠═══════════════\n║ Could not fetch\n║ video. Try HD.\n╚═══════════════")
	}
}

func handleInstagram(client *whatsmeow.Client, v *events.Message, url string) {
	if url == "" {
		msg := `╔═══════════════╗
║ 📸 INSTAGRAM
╠═══════════════
║ Usage:
║ .ig <url>
║
║ Example:
║ .ig https://
║ instagram.com/
║ p/xxxxx
╚═══════════════`
		replyMessage(client, v, msg)
		return
	}

	react(client, v.Info.Chat, v.Info.ID, "📸")
	
	msg := `╔═══════════════╗
║ 📸 PROCESSING
╠═══════════════
║ ⏳ Downloading
║ Please wait...
╚═══════════════`
	replyMessage(client, v, msg)

	type R struct {
		Data []struct {
			Url string `json:"url"`
		} `json:"data"`
	}
	var r R
	err := getJson("https://bk9.fun/downloader/instagram?url="+url, &r)
	
	if err == nil && len(r.Data) > 0 {
		sendVideo(client, v, r.Data[0].Url, "📸 *Instagram Video*\n✅ Successfully Downloaded")
	} else {
		replyMessage(client, v, "╔═══════════════╗\n║ ❌ FAILED\n╠═══════════════\n║ Private account\n║ or invalid link.\n╚═══════════════")
	}
}

func handlePinterest(client *whatsmeow.Client, v *events.Message, url string) {
	if url == "" {
		msg := `╔═══════════════╗
║ 📌 PINTEREST
╠═══════════════
║ Usage:
║ .pin <url>
╚═══════════════`
		replyMessage(client, v, msg)
		return
	}

	react(client, v.Info.Chat, v.Info.ID, "📌")
	
	msg := `╔═══════════════╗
║ 📌 PROCESSING
╠═══════════════
║ ⏳ Downloading
╚═══════════════`
	replyMessage(client, v, msg)

	type R struct {
		BK9    string `json:"BK9"`
		Status bool   `json:"status"`
	}
	var r R
	getJson("https://bk9.fun/downloader/pinterest?url="+url, &r)
	
	if r.BK9 != "" {
		sendImage(client, v, r.BK9, "📌 *Pinterest Image*\n✅ Downloaded")
	} else {
		replyMessage(client, v, "❌ Pinterest download failed.")
	}
}

func handleYouTubeMP3(client *whatsmeow.Client, v *events.Message, url string) {
	if url == "" {
		replyMessage(client, v, "⚠️ Please provide YouTube URL.")
		return
	}

	react(client, v.Info.Chat, v.Info.ID, "🎵")
	replyMessage(client, v, "⏳ *Downloading MP3...*")

	type R struct {
		BK9 struct {
			Mp3 string `json:"mp3"`
		} `json:"BK9"`
		Status bool `json:"status"`
	}
	var r R
	getJson("https://bk9.fun/downloader/youtube?url="+url, &r)
	
	if r.BK9.Mp3 != "" {
		sendDocument(client, v, r.BK9.Mp3, "audio.mp3", "audio/mpeg")
	} else {
		replyMessage(client, v, "❌ YouTube MP3 failed.")
	}
}

func handleYouTubeMP4(client *whatsmeow.Client, v *events.Message, url string) {
	if url == "" {
		replyMessage(client, v, "⚠️ Please provide YouTube URL.")
		return
	}

	react(client, v.Info.Chat, v.Info.ID, "📺")
	replyMessage(client, v, "⏳ *Downloading Video...*")

	type R struct {
		BK9 struct {
			Mp4 string `json:"mp4"`
		} `json:"BK9"`
		Status bool `json:"status"`
	}
	var r R
	getJson("https://bk9.fun/downloader/youtube?url="+url, &r)
	
	if r.BK9.Mp4 != "" {
		sendVideo(client, v, r.BK9.Mp4, "📺 *YouTube Video*\n✅ Downloaded")
	} else {
		replyMessage(client, v, "❌ YouTube MP4 failed.")
	}
}

// ==================== مددگار فنکشنز (Helpers) ====================

func getJson(url string, target interface{}) error {
	r, err := http.Get(url)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(target)
}

func sendVideo(client *whatsmeow.Client, v *events.Message, videoURL, caption string) {
	resp, err := http.Get(videoURL)
	if err != nil {
		fmt.Printf("❌ [VIDEO-ERR] Fetch failed: %v\n", err)
		return
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if len(data) == 0 { return }

	up, err := client.Upload(context.Background(), data, whatsmeow.MediaVideo)
	if err != nil { return }

	client.SendMessage(context.Background(), v.Info.Chat, &waProto.Message{
		VideoMessage: &waProto.VideoMessage{
			URL:           proto.String(up.URL),
			DirectPath:    proto.String(up.DirectPath),
			MediaKey:      up.MediaKey,
			Mimetype:      proto.String("video/mp4"),
			FileSHA256:    up.FileSHA256,
			FileEncSHA256: up.FileEncSHA256,
			FileLength:    proto.Uint64(uint64(len(data))), // ✅ Delivery Fix
			Caption:       proto.String(caption),
			ContextInfo: &waProto.ContextInfo{
				StanzaID:      proto.String(v.Info.ID),
				Participant:   proto.String(v.Info.Sender.String()),
				QuotedMessage: v.Message,
			},
		},
	})
}

func sendImage(client *whatsmeow.Client, v *events.Message, imageURL, caption string) {
	resp, err := http.Get(imageURL)
	if err != nil { return }
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	up, _ := client.Upload(context.Background(), data, whatsmeow.MediaImage)
	
	client.SendMessage(context.Background(), v.Info.Chat, &waProto.Message{
		ImageMessage: &waProto.ImageMessage{
			URL:           proto.String(up.URL),
			DirectPath:    proto.String(up.DirectPath),
			MediaKey:      up.MediaKey,
			Mimetype:      proto.String("image/jpeg"),
			FileSHA256:    up.FileSHA256,
			FileEncSHA256: up.FileEncSHA256,
			FileLength:    proto.Uint64(uint64(len(data))), // ✅ Delivery Fix
			Caption:       proto.String(caption),
			ContextInfo: &waProto.ContextInfo{
				StanzaID:      proto.String(v.Info.ID),
				Participant:   proto.String(v.Info.Sender.String()),
				QuotedMessage: v.Message,
			},
		},
	})
}

func sendDocument(client *whatsmeow.Client, v *events.Message, docURL, name, mime string) {
	resp, err := http.Get(docURL)
	if err != nil { return }
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	up, _ := client.Upload(context.Background(), data, whatsmeow.MediaDocument)
	
	client.SendMessage(context.Background(), v.Info.Chat, &waProto.Message{
		DocumentMessage: &waProto.DocumentMessage{
			URL:           proto.String(up.URL),
			DirectPath:    proto.String(up.DirectPath),
			MediaKey:      up.MediaKey,
			Mimetype:      proto.String(mime),
			FileName:      proto.String(name),
			FileSHA256:    up.FileSHA256,
			FileEncSHA256: up.FileEncSHA256,
			FileLength:    proto.Uint64(uint64(len(data))), // ✅ Delivery Fix
			Caption:       proto.String("✅ *Successfully Downloaded*"),
			ContextInfo: &waProto.ContextInfo{
				StanzaID:      proto.String(v.Info.ID),
				Participant:   proto.String(v.Info.Sender.String()),
				QuotedMessage: v.Message,
			},
		},
	})
}