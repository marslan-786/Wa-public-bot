package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types/events"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"google.golang.org/protobuf/proto"
)

// ==================== ٹولز سسٹم ====================
func handleSticker(client *whatsmeow.Client, v *events.Message) {
	react(client, v.Info.Chat, v.Info.ID, "🎨")
	
	msg := `╔═════════════════════╗
║   🎨 STICKER PROCESSING    
╠═════════════════════╣
║  ⏳ Creating sticker...    
║  Please wait...           
╚═════════════════════╝`
	replyMessage(client, v, msg)

	data, err := downloadMedia(client, v.Message)
	if err != nil {
		errMsg := `╔═════════════════╗
║  ❌ NO MEDIA FOUND       
╠═════════════════╣
║  Reply to an image or     
║  video to create sticker  
╚═════════════════╝`
		replyMessage(client, v, errMsg)
		return
	}

	ioutil.WriteFile("temp.jpg", data, 0644)
	exec.Command("ffmpeg", "-y", "-i", "temp.jpg", "-vcodec", "libwebp", "temp.webp").Run()
	b, _ := ioutil.ReadFile("temp.webp")
	up, _ := client.Upload(context.Background(), b, whatsmeow.MediaImage)

	client.SendMessage(context.Background(), v.Info.Chat, &waProto.Message{
		StickerMessage: &waProto.StickerMessage{
			URL:           proto.String(up.URL),
			DirectPath:    proto.String(up.DirectPath),
			MediaKey:      up.MediaKey,
			FileEncSHA256: up.FileEncSHA256,
			FileSHA256:    up.FileSHA256,
			Mimetype:      proto.String("image/webp"),
		},
	})

	os.Remove("temp.jpg")
	os.Remove("temp.webp")
}

func handleToImg(client *whatsmeow.Client, v *events.Message) {
	react(client, v.Info.Chat, v.Info.ID, "🖼️")
	
	msg := `╔══════════════════╗
║ 🖼️ IMAGE CONVERSION      
╠══════════════════╣
║ ⏳ Converting to image... 
║       Please wait...           
╚══════════════════╝`
	replyMessage(client, v, msg)  // اب msg صحیح ہے

	data, err := downloadMedia(client, v.Message)
	if err != nil {
		errMsg := `╔══════════════════╗
║  ❌ NO STICKER FOUND     
╠══════════════════╣
║  Reply to a sticker to    
║  convert it to image      
╚══════════════════╝`
		replyMessage(client, v, errMsg)
		return
	}

	ioutil.WriteFile("temp.webp", data, 0644)
	exec.Command("ffmpeg", "-y", "-i", "temp.webp", "temp.png").Run()
	b, _ := ioutil.ReadFile("temp.png")
	up, _ := client.Upload(context.Background(), b, whatsmeow.MediaImage)

	client.SendMessage(context.Background(), v.Info.Chat, &waProto.Message{
		ImageMessage: &waProto.ImageMessage{
			URL:           proto.String(up.URL),
			DirectPath:    proto.String(up.DirectPath),
			MediaKey:      up.MediaKey,
			FileEncSHA256: up.FileEncSHA256,
			FileSHA256:    up.FileSHA256,
			Mimetype:      proto.String("image/png"),
			Caption:       proto.String("✅ Converted to Image"),
			ContextInfo: &waProto.ContextInfo{
				StanzaID:      proto.String(v.Info.ID),
				Participant:   proto.String(v.Info.Sender.String()),
				QuotedMessage: v.Message,
			},
		},
	})

	os.Remove("temp.webp")
	os.Remove("temp.png")
}

func handleToVideo(client *whatsmeow.Client, v *events.Message) {
	react(client, v.Info.Chat, v.Info.ID, "🎥")
	
	msg := `╔═════════════════╗
║ 🎥 VIDEO CONVERSION      
╠═════════════════╣
║ ⏳ Converting to video... 
║       Please wait...           
╚═════════════════╝`
	replyMessage(client, v, msg)

	data, err := downloadMedia(client, v.Message)
	if err != nil {
		errMsg := `╔══════════════════╗
║  ❌ NO STICKER FOUND     
╠══════════════════╣
║  Reply to a sticker to    
║  convert it to video      
╚══════════════════╝`
		replyMessage(client, v, errMsg)
		return
	}

	ioutil.WriteFile("temp.webp", data, 0644)
	exec.Command("ffmpeg", "-y", "-i", "temp.webp", "temp.mp4").Run()
	d, _ := ioutil.ReadFile("temp.mp4")
	up, _ := client.Upload(context.Background(), d, whatsmeow.MediaVideo)

	client.SendMessage(context.Background(), v.Info.Chat, &waProto.Message{
		VideoMessage: &waProto.VideoMessage{
			URL:           proto.String(up.URL),
			DirectPath:    proto.String(up.DirectPath),
			MediaKey:      up.MediaKey,
			FileEncSHA256: up.FileEncSHA256,
			FileSHA256:    up.FileSHA256,
			Mimetype:      proto.String("video/mp4"),
			Caption:       proto.String("✅ Converted to Video"),
			ContextInfo: &waProto.ContextInfo{
				StanzaID:      proto.String(v.Info.ID),
				Participant:   proto.String(v.Info.Sender.String()),
				QuotedMessage: v.Message,
			},
		},
	})

	os.Remove("temp.webp")
	os.Remove("temp.mp4")
}

func handleRemoveBG(client *whatsmeow.Client, v *events.Message) {
	react(client, v.Info.Chat, v.Info.ID, "✂️")
	
	msg := `╔════════════════════╗
║ ✂️ BACKGROUND REMOVAL     
╠════════════════════╣
║  ⏳ Removing background... 
║          Please wait...           
╚════════════════════╝`
	replyMessage(client, v, msg)

	d, err := downloadMedia(client, v.Message)
	if err != nil {
		errMsg := `╔═════════════════╗
║  ❌ NO IMAGE FOUND       
╠═════════════════╣
║  Reply to an image to     
║  remove background        
╚═════════════════╝`
		replyMessage(client, v, errMsg)
		return
	}

	u := uploadToCatbox(d)
	imgURL := "https://bk9.fun/tools/removebg?url=" + u

	r, _ := http.Get(imgURL)
	imgData, _ := ioutil.ReadAll(r.Body)
	up, _ := client.Upload(context.Background(), imgData, whatsmeow.MediaImage)

	client.SendMessage(context.Background(), v.Info.Chat, &waProto.Message{
		ImageMessage: &waProto.ImageMessage{
			URL:           proto.String(up.URL),
			DirectPath:    proto.String(up.DirectPath),
			MediaKey:      up.MediaKey,
			FileEncSHA256: up.FileEncSHA256,
			FileSHA256:    up.FileSHA256,
			Mimetype:      proto.String("image/png"),
			Caption:       proto.String("✂️ Background Removed\n\n✅ Successfully Processed"),
			ContextInfo: &waProto.ContextInfo{
				StanzaID:      proto.String(v.Info.ID),
				Participant:   proto.String(v.Info.Sender.String()),
				QuotedMessage: v.Message,
			},
		},
	})
}

func handleRemini(client *whatsmeow.Client, v *events.Message) {
	react(client, v.Info.Chat, v.Info.ID, "✨")
	
	msg := `╔═══════════════════╗
║ ✨ IMAGE ENHANCEMENT     
╠═══════════════════╣
║  ⏳ Enhancing image...     
║       Please wait...           
╚═══════════════════╝`
	replyMessage(client, v, msg)

	d, err := downloadMedia(client, v.Message)
	if err != nil {
		errMsg := `╔════════════════╗
║ ❌ NO IMAGE FOUND       
╠════════════════╣
║  Reply to an image to     
║  enhance quality          
╚════════════════╝`
		replyMessage(client, v, errMsg)
		return
	}

	u := uploadToCatbox(d)
	type R struct {
		Url string `json:"url"`
	}
	var r R
	getJson("https://remini.mobilz.pw/enhance?url="+u, &r)

	if r.Url != "" {
		resp, _ := http.Get(r.Url)
		imgData, _ := ioutil.ReadAll(resp.Body)
		up, _ := client.Upload(context.Background(), imgData, whatsmeow.MediaImage)

		client.SendMessage(context.Background(), v.Info.Chat, &waProto.Message{
			ImageMessage: &waProto.ImageMessage{
				URL:           proto.String(up.URL),
				DirectPath:    proto.String(up.DirectPath),
				MediaKey:      up.MediaKey,
				FileEncSHA256: up.FileEncSHA256,
				FileSHA256:    up.FileSHA256,
				Mimetype:      proto.String("image/jpeg"),
				Caption:       proto.String("✨ Enhanced Image\n\n✅ Quality Improved"),
				ContextInfo: &waProto.ContextInfo{
					StanzaID:      proto.String(v.Info.ID),
					Participant:   proto.String(v.Info.Sender.String()),
					QuotedMessage: v.Message,
			},
			},
		})
	} else {
		errMsg := `╔═══════════════════╗
║ ❌ ENHANCEMENT FAILED   
╠═══════════════════╣
║  Could not enhance image  
║       Please try again         
╚═══════════════════╝`
		replyMessage(client, v, errMsg)
	}
}

func handleToURL(client *whatsmeow.Client, v *events.Message) {
	react(client, v.Info.Chat, v.Info.ID, "🔗")
	
	msg := `╔══════════════════╗
║  🔗 UPLOADING MEDIA       
╠══════════════════╣
║ ⏳ Uploading to server... 
║         Please wait...           
╚══════════════════╝`
	replyMessage(client, v, msg)

	d, err := downloadMedia(client, v.Message)
	if err != nil {
		errMsg := `╔═════════════════╗
║  ❌ NO MEDIA FOUND       
╠═══════════════════╣
║ Reply to media to get URL
╚═══════════════════╝`
		replyMessage(client, v, errMsg)
		return
	}

	uploadURL := uploadToCatbox(d)
	
	resultMsg := fmt.Sprintf(`╔═════════════════╗
║  🔗 MEDIA UPLOADED        
╠═════════════════╣
║                           
║  📎 *Direct Link:*        
║  %s                       
║                           
║ ✅ *Successfully Uploaded*
║                           
╚═══════════════════╝`, uploadURL)

	replyMessage(client, v, resultMsg)
}

func handleWeather(client *whatsmeow.Client, v *events.Message, city string) {
	if city == "" {
		msg := `╔════════════════════╗
║🌤️ WEATHER INFORMATION   
╠════════════════════╣
║                           
║  Usage:                   
║  .weather <city>          
║                           
║  Example:                 
║  .weather Karachi         
║             .weather London          
║                           
╚════════════════════╝`
		replyMessage(client, v, msg)
		return
	}

	react(client, v.Info.Chat, v.Info.ID, "🌦️")
	
	r, err := http.Get("https://wttr.in/" + city + "?format=%C+%t")
	if err != nil {
		errMsg := `╔═══════════════════╗
║❌ WEATHER FETCH FAILED 
╠═══════════════════╣
║   Could not get weather    
║   Please check city name   
╚═══════════════════╝`
		replyMessage(client, v, errMsg)
		return
	}

	d, _ := ioutil.ReadAll(r.Body)
	weatherInfo := string(d)

	msg := fmt.Sprintf(`╔═══════════════╗
║ 🌤️ WEATHER INFO          
╠═══════════════╣
║                           
║  📍 *City:* %s            
║  🌡️ *Info:* %s            
║                           
╚═══════════════╝`, city, weatherInfo)

	replyMessage(client, v, msg)
}

func handleTranslate(client *whatsmeow.Client, v *events.Message, args []string) {
	react(client, v.Info.Chat, v.Info.ID, "🌍")

	t := strings.Join(args, " ")
	if t == "" {
		if v.Message.ExtendedTextMessage != nil {
			q := v.Message.ExtendedTextMessage.GetContextInfo().GetQuotedMessage()
			if q != nil {
				t = q.GetConversation()
			}
		}
	}

	if t == "" {
		msg := `╔══════════════╗
║   🌍 TRANSLATOR            
╠══════════════╣
║                           
║  Usage:                   
║  .tr <text>               
║                           
║  Or reply to message with:
║  .tr                      
║                           
╚═══════════════════╝`
		replyMessage(client, v, msg)
		return
	}

	r, _ := http.Get(fmt.Sprintf("https://translate.googleapis.com/translate_a/single?client=gtx&sl=auto&tl=ur&dt=t&q=%s", url.QueryEscape(t)))
	var res []interface{}
	json.NewDecoder(r.Body).Decode(&res)

	if len(res) > 0 {
		translated := res[0].([]interface{})[0].([]interface{})[0].(string)
		msg := fmt.Sprintf(`╔═══════════════════╗
║ 🌍 TRANSLATION RESULT    
╠═══════════════════╣
║                           
║  📝 *Original:*           
║  %s                       
║                           
║  📝 *Translated:*         
║  %s                       
║                           
╚════════════════════╝`, t, translated)

		replyMessage(client, v, msg)
	} else {
		errMsg := `╔══════════════════╗
║ ❌ TRANSLATION FAILED    
╠══════════════════╣
║  Could not translate text 
║  Please try again         
╚══════════════════╝`
		replyMessage(client, v, errMsg)
	}
}

func handleVV(client *whatsmeow.Client, v *events.Message) {
	react(client, v.Info.Chat, v.Info.ID, "🫣")

	if v.Message.ExtendedTextMessage == nil {
		msg := `╔════════════════╗
║    ⚠️ ViewOnce       
╠════════════════╣
║  Reply to any media      
║  message to send it     
╚════════════════╝`
		replyMessage(client, v, msg)
		return
	}

	quoted := v.Message.ExtendedTextMessage.GetContextInfo().GetQuotedMessage()
	if quoted == nil {
		msg := `╔═══════════════════╗
║ ❌ NO MESSAGE FOUND     
╠═══════════════════╣
║  Reply to a media message  
╚═══════════════════╝`
		replyMessage(client, v, msg)
		return
	}

	data, err := downloadMedia(client, quoted)

	if err != nil {
		errMsg := `╔═════════════════╗
║ ❌ DOWNLOAD FAILED       
╠════════════════════╣
║  Could not send media
╚════════════════════╝`
		replyMessage(client, v, errMsg)
		return
	}

	// Check for image
	if quoted.ImageMessage != nil || 
	   (quoted.ViewOnceMessage != nil && quoted.ViewOnceMessage.Message.ImageMessage != nil) ||
	   (quoted.ViewOnceMessageV2 != nil && quoted.ViewOnceMessageV2.Message.ImageMessage != nil) {
		up, _ := client.Upload(context.Background(), data, whatsmeow.MediaImage)
		client.SendMessage(context.Background(), v.Info.Chat, &waProto.Message{
			ImageMessage: &waProto.ImageMessage{
				URL:           proto.String(up.URL),
				DirectPath:    proto.String(up.DirectPath),
				MediaKey:      up.MediaKey,
				FileEncSHA256: up.FileEncSHA256,
				FileSHA256:    up.FileSHA256,
				Mimetype:      proto.String("image/jpeg"),
				Caption:       proto.String("🫣 Media Copied\n\n✅ Successfully Retrieved"),
				ContextInfo: &waProto.ContextInfo{
					StanzaID:      proto.String(v.Info.ID),
					Participant:   proto.String(v.Info.Sender.String()),
					QuotedMessage: v.Message,
				},
			},
		})
	} else if quoted.VideoMessage != nil || 
	          (quoted.ViewOnceMessage != nil && quoted.ViewOnceMessage.Message.VideoMessage != nil) ||
	          (quoted.ViewOnceMessageV2 != nil && quoted.ViewOnceMessageV2.Message.VideoMessage != nil) {
		up, _ := client.Upload(context.Background(), data, whatsmeow.MediaVideo)
		client.SendMessage(context.Background(), v.Info.Chat, &waProto.Message{
			VideoMessage: &waProto.VideoMessage{
				URL:           proto.String(up.URL),
				DirectPath:    proto.String(up.DirectPath),
				MediaKey:      up.MediaKey,
				FileEncSHA256: up.FileEncSHA256,
				FileSHA256:    up.FileSHA256,
				Mimetype:      proto.String("video/mp4"),
				Caption:       proto.String("🫣 Media Copied\n\n✅ Successfully Retrieved"),
				ContextInfo: &waProto.ContextInfo{
					StanzaID:      proto.String(v.Info.ID),
					Participant:   proto.String(v.Info.Sender.String()),
					QuotedMessage: v.Message,
				},
			},
		})
	} else {
		msg := `╔═══════════════════╗
║ ❌ NO MEDIA FOUND     
╠═══════════════════╣
║  Reply to image/video  
╚═══════════════════╝`
		replyMessage(client, v, msg)
	}
}

// ==================== میڈیا ہیلپرز ====================
func downloadMedia(client *whatsmeow.Client, m *waProto.Message) ([]byte, error) {
	var d whatsmeow.DownloadableMessage
	if m.ImageMessage != nil {
		d = m.ImageMessage
	} else if m.VideoMessage != nil {
		d = m.VideoMessage
	} else if m.DocumentMessage != nil {
		d = m.DocumentMessage
	} else if m.StickerMessage != nil {
		d = m.StickerMessage
	} else if m.ExtendedTextMessage != nil && m.ExtendedTextMessage.ContextInfo != nil {
		q := m.ExtendedTextMessage.ContextInfo.QuotedMessage
		if q != nil {
			if q.ImageMessage != nil {
				d = q.ImageMessage
			} else if q.VideoMessage != nil {
				d = q.VideoMessage
			} else if q.StickerMessage != nil {
				d = q.StickerMessage
			}
		}
	}
	if d == nil {
		return nil, fmt.Errorf("no media")
	}
	return client.Download(context.Background(), d)
}

func uploadToCatbox(d []byte) string {
	b := new(bytes.Buffer)
	w := multipart.NewWriter(b)
	p, _ := w.CreateFormFile("fileToUpload", "f.jpg")
	p.Write(d)
	w.WriteField("reqtype", "fileupload")
	w.Close()
	r, _ := http.Post("https://catbox.moe/user/api.php", w.FormDataContentType(), b)
	res, _ := ioutil.ReadAll(r.Body)
	return string(res)
}