package main

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types/events"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"google.golang.org/protobuf/proto"
)

// ==================== ڈاؤن لوڈر سسٹم ====================
func handleTikTok(client *whatsmeow.Client, v *events.Message, url string) {
	if url == "" {
		msg := `╔═══════════════════════════╗
║    📝 TIKTOK DOWNLOADER    ║
╠═══════════════════════════╣
║                           ║
║  Usage:                   ║
║  .tiktok <url>            ║
║                           ║
║  Example:                 ║
║  .tiktok https://vm.tiktok║
║  .com/xxxxx               ║
║                           ║
╚═══════════════════════════╝`
		replyMessage(client, v, msg)
		return
	}

	react(client, v.Info.Chat, v.Info.ID, "🎵")
	
	msg := `╔═══════════════════════════╗
║    🎵 TIKTOK PROCESSING    ║
╠═══════════════════════════╣
║  ⏳ Downloading video...  ║
║  Please wait...           ║
╚═══════════════════════════╝`
	replyMessage(client, v, msg)

	type R struct {
		Data struct {
			Play string `json:"play"`
		} `json:"data"`
	}
	var r R
	getJson("https://www.tikwm.com/api/?url="+url, &r)
	
	if r.Data.Play != "" {
		sendVideo(client, v, r.Data.Play, "🎵 TikTok Video\n\n✅ Downloaded Successfully")
	} else {
		errMsg := `╔═══════════════════════════╗
║    ❌ DOWNLOAD FAILED      ║
╠═══════════════════════════╣
║  Could not download video ║
║  Please check the URL     ║
╚═══════════════════════════╝`
		replyMessage(client, v, errMsg)
	}
}

func handleFacebook(client *whatsmeow.Client, v *events.Message, url string) {
	if url == "" {
		msg := `╔═══════════════════════════╗
║   📘 FACEBOOK DOWNLOADER   ║
╠═══════════════════════════╣
║                           ║
║  Usage:                   ║
║  .fb <url>                ║
║                           ║
║  Example:                 ║
║  .fb https://fb.watch/xxxx║
║                           ║
╚═══════════════════════════╝`
		replyMessage(client, v, msg)
		return
	}

	react(client, v.Info.Chat, v.Info.ID, "📘")
	
	msg := `╔═══════════════════════════╗
║   📘 FACEBOOK PROCESSING   ║
╠═══════════════════════════╣
║  ⏳ Downloading video...  ║
║  Please wait...           ║
╚═══════════════════════════╝`
	replyMessage(client, v, msg)

	type R struct {
		BK9 struct {
			HD string `json:"HD"`
		} `json:"BK9"`
		Status bool `json:"status"`
	}
	var r R
	getJson("https://bk9.fun/downloader/facebook?url="+url, &r)
	
	if r.Status {
		sendVideo(client, v, r.BK9.HD, "📘 Facebook Video\n\n✅ Downloaded Successfully")
	} else {
		errMsg := `╔═══════════════════════════╗
║    ❌ DOWNLOAD FAILED      ║
╠═══════════════════════════╣
║  Could not download video ║
║  Please check the URL     ║
╚═══════════════════════════╝`
		replyMessage(client, v, errMsg)
	}
}

func handleInstagram(client *whatsmeow.Client, v *events.Message, url string) {
	if url == "" {
		msg := `╔═══════════════════════════╗
║  📸 INSTAGRAM DOWNLOADER   ║
╠═══════════════════════════╣
║                           ║
║  Usage:                   ║
║  .ig <url>                ║
║                           ║
║  Example:                 ║
║  .ig https://instagram.com║
║  /p/xxxxx                 ║
║                           ║
╚═══════════════════════════╝`
		replyMessage(client, v, msg)
		return
	}

	react(client, v.Info.Chat, v.Info.ID, "📸")
	
	msg := `╔═══════════════════════════╗
║  📸 INSTAGRAM PROCESSING   ║
╠═══════════════════════════╣
║  ⏳ Downloading content... ║
║  Please wait...           ║
╚═══════════════════════════╝`
	replyMessage(client, v, msg)

	type R struct {
		Video struct {
			Url string `json:"url"`
		} `json:"video"`
	}
	var r R
	getJson("https://api.tiklydown.eu.org/api/download?url="+url, &r)
	
	if r.Video.Url != "" {
		sendVideo(client, v, r.Video.Url, "📸 Instagram Video\n\n✅ Downloaded Successfully")
	} else {
		errMsg := `╔═══════════════════════════╗
║    ❌ DOWNLOAD FAILED      ║
╠═══════════════════════════╣
║  Could not download video ║
║  Please check the URL     ║
╚═══════════════════════════╝`
		replyMessage(client, v, errMsg)
	}
}

func handlePinterest(client *whatsmeow.Client, v *events.Message, url string) {
	if url == "" {
		msg := `╔═══════════════════════════╗
║  📌 PINTEREST DOWNLOADER   ║
╠═══════════════════════════╣
║                           ║
║  Usage:                   ║
║  .pin <url>               ║
║                           ║
║  Example:                 ║
║  .pin https://pin.it/xxxxx║
║                           ║
╚═══════════════════════════╝`
		replyMessage(client, v, msg)
		return
	}

	react(client, v.Info.Chat, v.Info.ID, "📌")
	
	msg := `╔═══════════════════════════╗
║  📌 PINTEREST PROCESSING   ║
╠═══════════════════════════╣
║  ⏳ Downloading image...   ║
║  Please wait...           ║
╚═══════════════════════════╝`
	replyMessage(client, v, msg)

	type R struct {
		BK9 struct {
			Url string `json:"url"`
		} `json:"BK9"`
		Status bool `json:"status"`
	}
	var r R
	getJson("https://bk9.fun/downloader/pinterest?url="+url, &r)
	
	if r.Status {
		sendImage(client, v, r.BK9.Url, "📌 Pinterest Image\n\n✅ Downloaded Successfully")
	} else {
		errMsg := `╔═══════════════════════════╗
║    ❌ DOWNLOAD FAILED      ║
╠═══════════════════════════╣
║  Could not download image ║
║  Please check the URL     ║
╚═══════════════════════════╝`
		replyMessage(client, v, errMsg)
	}
}

func handleYouTubeMP3(client *whatsmeow.Client, v *events.Message, url string) {
	if url == "" {
		msg := `╔═══════════════════════════╗
║   🎵 YOUTUBE MP3 DOWNLOADER║
╠═══════════════════════════╣
║                           ║
║  Usage:                   ║
║  .ytmp3 <url>             ║
║                           ║
║  Example:                 ║
║  .ytmp3 https://youtu.be/ ║
║  xxxxx                    ║
║                           ║
╚═══════════════════════════╝`
		replyMessage(client, v, msg)
		return
	}

	react(client, v.Info.Chat, v.Info.ID, "🎵")
	
	msg := `╔═══════════════════════════╗
║  🎵 YOUTUBE MP3 PROCESSING ║
╠═══════════════════════════╣
║  ⏳ Downloading audio...   ║
║  Please wait...           ║
╚═══════════════════════════╝`
	replyMessage(client, v, msg)

	type R struct {
		BK9 struct {
			Mp3 string `json:"mp3"`
		} `json:"BK9"`
		Status bool `json:"status"`
	}
	var r R
	getJson("https://bk9.fun/downloader/youtube?url="+url, &r)
	
	if r.Status {
		sendDocument(client, v, r.BK9.Mp3, "audio.mp3", "audio/mpeg")
	} else {
		errMsg := `╔═══════════════════════════╗
║    ❌ DOWNLOAD FAILED      ║
╠═══════════════════════════╣
║  Could not download audio ║
║  Please check the URL     ║
╚═══════════════════════════╝`
		replyMessage(client, v, errMsg)
	}
}

func handleYouTubeMP4(client *whatsmeow.Client, v *events.Message, url string) {
	if url == "" {
		msg := `╔═══════════════════════════╗
║   📺 YOUTUBE MP4 DOWNLOADER║
╠═══════════════════════════╣
║                           ║
║  Usage:                   ║
║  .ytmp4 <url>             ║
║                           ║
║  Example:                 ║
║  .ytmp4 https://youtu.be/ ║
║  xxxxx                    ║
║                           ║
╚═══════════════════════════╝`
		replyMessage(client, v, msg)
		return
	}

	react(client, v.Info.Chat, v.Info.ID, "📺")
	
	msg := `╔═══════════════════════════╗
║  📺 YOUTUBE MP4 PROCESSING ║
╠═══════════════════════════╣
║  ⏳ Downloading video...   ║
║  Please wait...           ║
╚═══════════════════════════╝`
	replyMessage(client, v, msg)

	type R struct {
		BK9 struct {
			Mp4 string `json:"mp4"`
		} `json:"BK9"`
		Status bool `json:"status"`
	}
	var r R
	getJson("https://bk9.fun/downloader/youtube?url="+url, &r)
	
	if r.Status {
		sendVideo(client, v, r.BK9.Mp4, "📺 YouTube Video\n\n✅ Downloaded Successfully")
	} else {
		errMsg := `╔═══════════════════════════╗
║    ❌ DOWNLOAD FAILED      ║
╠═══════════════════════════╣
║  Could not download video ║
║  Please check the URL     ║
╚═══════════════════════════╝`
		replyMessage(client, v, errMsg)
	}
}

// ==================== HELPER FUNCTIONS ====================
func getJson(url string, target interface{}) error {
	r, err := http.Get(url)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(target)
}

func sendVideo(client *whatsmeow.Client, v *events.Message, url, caption string) {
	r, err := http.Get(url)
	if err != nil {
		replyMessage(client, v, "❌ Failed to download video")
		return
	}
	d, _ := ioutil.ReadAll(r.Body)
	up, _ := client.Upload(context.Background(), d, whatsmeow.MediaVideo)
	
	client.SendMessage(context.Background(), v.Info.Chat, &waProto.Message{
		VideoMessage: &waProto.VideoMessage{
			URL:           proto.String(up.URL),
			DirectPath:    proto.String(up.DirectPath),
			MediaKey:      up.MediaKey,
			FileEncSHA256: up.FileEncSHA256,
			FileSHA256:    up.FileSHA256,
			Mimetype:      proto.String("video/mp4"),
			Caption:       proto.String(caption),
			ContextInfo: &waProto.ContextInfo{
				StanzaID:      proto.String(v.Info.ID),
				Participant:   proto.String(v.Info.Sender.String()),
				QuotedMessage: v.Message,
			},
		},
	})
}

func sendImage(client *whatsmeow.Client, v *events.Message, url, caption string) {
	r, err := http.Get(url)
	if err != nil {
		replyMessage(client, v, "❌ Failed to download image")
		return
	}
	d, _ := ioutil.ReadAll(r.Body)
	up, _ := client.Upload(context.Background(), d, whatsmeow.MediaImage)
	
	client.SendMessage(context.Background(), v.Info.Chat, &waProto.Message{
		ImageMessage: &waProto.ImageMessage{
			URL:           proto.String(up.URL),
			DirectPath:    proto.String(up.DirectPath),
			MediaKey:      up.MediaKey,
			FileEncSHA256: up.FileEncSHA256,
			FileSHA256:    up.FileSHA256,
			Mimetype:      proto.String("image/jpeg"),
			Caption:       proto.String(caption),
			ContextInfo: &waProto.ContextInfo{
				StanzaID:      proto.String(v.Info.ID),
				Participant:   proto.String(v.Info.Sender.String()),
				QuotedMessage: v.Message,
			},
		},
	})
}

func sendDocument(client *whatsmeow.Client, v *events.Message, url, name, mime string) {
	r, err := http.Get(url)
	if err != nil {
		replyMessage(client, v, "❌ Failed to download document")
		return
	}
	d, _ := ioutil.ReadAll(r.Body)
	up, _ := client.Upload(context.Background(), d, whatsmeow.MediaDocument)
	
	client.SendMessage(context.Background(), v.Info.Chat, &waProto.Message{
		DocumentMessage: &waProto.DocumentMessage{
			URL:           proto.String(up.URL),
			DirectPath:    proto.String(up.DirectPath),
			MediaKey:      up.MediaKey,
			FileEncSHA256: up.FileEncSHA256,
			FileSHA256:    up.FileSHA256,
			Mimetype:      proto.String(mime),
			FileName:      proto.String(name),
			Caption:       proto.String("✅ Downloaded Successfully"),
			ContextInfo: &waProto.ContextInfo{
				StanzaID:      proto.String(v.Info.ID),
				Participant:   proto.String(v.Info.Sender.String()),
				QuotedMessage: v.Message,
			},
		},
	})
}