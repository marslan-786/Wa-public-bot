package plugins

import (
	"fmt"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types/events"
)

func init() {
	// Instagram Downloader
	Register("ig", "Downloaders", func(cli *whatsmeow.Client, evt *events.Message, args []string) {
		if len(args) == 0 { return }
		fmt.Println("Downloading Instagram:", args[0])
		// اے پی آئی کال اور واٹس ایپ سینڈنگ لاجک یہاں آئے گی
	})

	// TikTok Downloader
	Register("tiktok", "Downloaders", func(cli *whatsmeow.Client, evt *events.Message, args []string) {
		// ٹک ٹاک لاجک
	})

	// Facebook, Pinterest, YouTube MP3/MP4
}