package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/image/draw"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	waProto "go.mau.fi/whatsmeow/binary/proto"
)

// -------------------------
// Helpers
// -------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func mustPOST(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return false
	}
	return true
}

func getIntQuery(r *http.Request, key string, def int) int {
	v := strings.TrimSpace(r.URL.Query().Get(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func getBotByID(w http.ResponseWriter, botID string) (*whatsmeow.Client, bool) {
	botID = strings.TrimSpace(botID)
	if botID == "" {
		http.Error(w, "bot_id missing", http.StatusBadRequest)
		return nil, false
	}
	clientsMutex.RLock()
	bot, ok := activeClients[botID]
	clientsMutex.RUnlock()
	if !ok || bot == nil {
		http.Error(w, "Bot offline", http.StatusNotFound)
		return nil, false
	}
	if bot.Store == nil || bot.Store.ID == nil || !bot.IsLoggedIn() {
		http.Error(w, "Bot not logged in", http.StatusBadRequest)
		return nil, false
	}
	return bot, true
}

func parseJIDOr400(w http.ResponseWriter, s string) (types.JID, bool) {
	jid, err := types.ParseJID(strings.TrimSpace(s))
	if err != nil {
		http.Error(w, "invalid jid", http.StatusBadRequest)
		return types.EmptyJID, false
	}
	return jid, true
}

// -------------------------
// V2: Health / Version
// -------------------------

func handleHealthV2(w http.ResponseWriter, r *http.Request) {
	clientsMutex.RLock()
	count := len(activeClients)
	clientsMutex.RUnlock()

	writeJSON(w, 200, map[string]any{
		"ok":        true,
		"time":      time.Now().UTC().Format(time.RFC3339),
		"bots_live": count,
	})
}

func handleVersionV2(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{
		"service": "impossible-bot",
		"time":    time.Now().UTC().Format(time.RFC3339),
	})
}

// -------------------------
// V2: Bots
// -------------------------

func handleBotListV2(w http.ResponseWriter, r *http.Request) {
	clientsMutex.RLock()
	defer clientsMutex.RUnlock()

	out := make([]map[string]any, 0, len(activeClients))
	for id, c := range activeClients {
		jid := ""
		logged := false
		if c != nil {
			logged = c.IsLoggedIn()
			if c.Store != nil && c.Store.ID != nil {
				jid = c.Store.ID.String()
			}
		}
		out = append(out, map[string]any{
			"bot_id":     id,
			"is_logged":  logged,
			"jid":        jid,
			"is_connected": func() bool { if c == nil { return false }; return c.IsConnected() }(),
		})
	}
	writeJSON(w, 200, map[string]any{"ok": true, "bots": out})
}

func handleBotOnlineV2(w http.ResponseWriter, r *http.Request) {
	botID := r.URL.Query().Get("bot_id")
	_, ok := getBotByID(w, botID)
	if !ok {
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "online": true})
}

// -------------------------
// DB: chat_history utilities
// -------------------------

// NOTE: Your initHistoryDB creates table: chat_history (NOT messages)
const historyTable = "chat_history"

func dbReady(w http.ResponseWriter) bool {
	if historyDB == nil {
		http.Error(w, "MySQL disconnected", 500)
		return false
	}
	return true
}

// -------------------------
// V2: Chats
// -------------------------

// GET /api/v2/chats/list?bot_id=xxx&limit=30&offset=0
// WhatsApp-like: last message per chat
func handleChatsListV2(w http.ResponseWriter, r *http.Request) {
	if !dbReady(w) {
		return
	}
	botID := r.URL.Query().Get("bot_id")
	_, ok := getBotByID(w, botID)
	if !ok {
		return
	}

	limit := getIntQuery(r, "limit", 30)
	offset := getIntQuery(r, "offset", 0)

	// last msg per chat
	q := fmt.Sprintf(`
SELECT h.chat_id, h.sender, h.sender_name, h.message_id, h.timestamp, h.msg_type, h.content, h.is_from_me, h.is_group, h.is_sticker
FROM %s h
JOIN (
  SELECT chat_id, MAX(timestamp) AS max_ts
  FROM %s
  WHERE bot_id = ?
  GROUP BY chat_id
) t ON t.chat_id = h.chat_id AND t.max_ts = h.timestamp
WHERE h.bot_id = ?
ORDER BY h.timestamp DESC
LIMIT ? OFFSET ?;`, historyTable, historyTable)

	rows, err := historyDB.Query(q, botID, botID, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	type ChatPreview struct {
		ChatID     string `json:"chat_id"`
		IsGroup    bool   `json:"is_group"`
		LastType   string `json:"last_type"`
		LastText   string `json:"last_text"`
		LastFromMe bool   `json:"last_from_me"`
		LastAt     string `json:"last_at"`
		Sender     string `json:"sender"`
		SenderName string `json:"sender_name"`
		MessageID  string `json:"message_id"`
		IsSticker  bool   `json:"is_sticker"`
		Unread     int    `json:"unread"` // later
	}

	items := []ChatPreview{}
	for rows.Next() {
		var chatID, sender, senderName, msgID, msgType, content string
		var ts time.Time
		var isFromMe, isGroup, isSticker bool

		if err := rows.Scan(&chatID, &sender, &senderName, &msgID, &ts, &msgType, &content, &isFromMe, &isGroup, &isSticker); err != nil {
			continue
		}

		items = append(items, ChatPreview{
			ChatID:     chatID,
			IsGroup:    isGroup || strings.Contains(chatID, "@g.us"),
			LastType:   msgType,
			LastText:   content,
			LastFromMe: isFromMe,
			LastAt:     ts.UTC().Format(time.RFC3339),
			Sender:     sender,
			SenderName: senderName,
			MessageID:  msgID,
			IsSticker:  isSticker,
			Unread:     0,
		})
	}

	writeJSON(w, 200, map[string]any{"ok": true, "bot_id": botID, "items": items})
}

// GET /api/v2/chats/search?bot_id=xxx&q=hi&limit=50
func handleChatsSearchV2(w http.ResponseWriter, r *http.Request) {
	if !dbReady(w) {
		return
	}
	botID := r.URL.Query().Get("bot_id")
	_, ok := getBotByID(w, botID)
	if !ok {
		return
	}
	qText := strings.TrimSpace(r.URL.Query().Get("q"))
	if qText == "" {
		http.Error(w, "q missing", 400)
		return
	}
	limit := getIntQuery(r, "limit", 50)

	q := fmt.Sprintf(`
SELECT id, bot_id, chat_id, sender, sender_name, message_id, timestamp, msg_type, content, is_from_me, is_group, quoted_msg, is_sticker
FROM %s
WHERE bot_id = ? AND content LIKE ?
ORDER BY timestamp DESC
LIMIT ?;`, historyTable)

	rows, err := historyDB.Query(q, botID, "%"+qText+"%", limit)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	var out []ChatMessage
	for rows.Next() {
		var m ChatMessage
		if err := rows.Scan(&m.ID, &m.BotID, &m.ChatID, &m.Sender, &m.SenderName, &m.MessageID, &m.Timestamp, &m.Type, &m.Content, &m.IsFromMe, &m.IsGroup, &m.QuotedMsg, &m.IsSticker); err == nil {
			out = append(out, m)
		}
	}

	writeJSON(w, 200, map[string]any{"ok": true, "bot_id": botID, "q": qText, "items": out})
}

func handleChatOpenV2(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]any{"ok": true}) }
func handleChatMuteV2(w http.ResponseWriter, r *http.Request) { writeJSON(w, 501, map[string]any{"ok": false, "error": "not implemented"}) }
func handleChatPinV2(w http.ResponseWriter, r *http.Request) { writeJSON(w, 501, map[string]any{"ok": false, "error": "not implemented"}) }
func handleChatArchiveV2(w http.ResponseWriter, r *http.Request) { writeJSON(w, 501, map[string]any{"ok": false, "error": "not implemented"}) }
func handleChatClearV2(w http.ResponseWriter, r *http.Request) { writeJSON(w, 501, map[string]any{"ok": false, "error": "not implemented"}) }

// -------------------------
// V2: Messages
// -------------------------

// GET /api/v2/messages/history?bot_id=xxx&chat_id=jid&limit=50&before_id=123
func handleMessagesHistoryV2(w http.ResponseWriter, r *http.Request) {
	if !dbReady(w) {
		return
	}
	botID := r.URL.Query().Get("bot_id")
	_, ok := getBotByID(w, botID)
	if !ok {
		return
	}

	chatID := strings.TrimSpace(r.URL.Query().Get("chat_id"))
	if chatID == "" {
		http.Error(w, "chat_id missing", 400)
		return
	}

	limit := getIntQuery(r, "limit", 50)
	beforeID := strings.TrimSpace(r.URL.Query().Get("before_id"))

	base := fmt.Sprintf(`
SELECT id, bot_id, chat_id, sender, sender_name, message_id, timestamp, msg_type, content, is_from_me, is_group, quoted_msg, is_sticker
FROM %s
WHERE bot_id = ? AND chat_id = ?`, historyTable)

	args := []any{botID, chatID}

	if beforeID != "" {
		if id, err := strconv.ParseInt(beforeID, 10, 64); err == nil {
			base += " AND id < ?"
			args = append(args, id)
		}
	}

	base += " ORDER BY id DESC LIMIT ?"
	args = append(args, limit)

	rows, err := historyDB.Query(base, args...)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	var out []ChatMessage
	for rows.Next() {
		var m ChatMessage
		if err := rows.Scan(&m.ID, &m.BotID, &m.ChatID, &m.Sender, &m.SenderName, &m.MessageID, &m.Timestamp, &m.Type, &m.Content, &m.IsFromMe, &m.IsGroup, &m.QuotedMsg, &m.IsSticker); err == nil {
			out = append(out, m)
		}
	}

	writeJSON(w, 200, map[string]any{"ok": true, "bot_id": botID, "chat_id": chatID, "items": out})
}

// NOTE: send text/media/status/profile/contact already exist in main.go as V2 real handlers.
// We keep these as "not implemented here" to avoid duplicate symbols.
func handleMessageReplyV2(w http.ResponseWriter, r *http.Request)   { writeJSON(w, 501, map[string]any{"ok": false, "error": "reply not implemented yet"}) }
func handleMessageForwardV2(w http.ResponseWriter, r *http.Request) { writeJSON(w, 501, map[string]any{"ok": false, "error": "forward not implemented yet"}) }
func handleMessageReactV2(w http.ResponseWriter, r *http.Request)   { writeJSON(w, 501, map[string]any{"ok": false, "error": "react not implemented yet"}) }
func handleMessageStarV2(w http.ResponseWriter, r *http.Request)    { writeJSON(w, 501, map[string]any{"ok": false, "error": "star not implemented"}) }
func handleMessagePinV2(w http.ResponseWriter, r *http.Request)     { writeJSON(w, 501, map[string]any{"ok": false, "error": "pin not implemented"}) }
func handleMessageDeleteV2(w http.ResponseWriter, r *http.Request)  { writeJSON(w, 501, map[string]any{"ok": false, "error": "delete not implemented"}) }
func handleMessageEditV2(w http.ResponseWriter, r *http.Request)    { writeJSON(w, 501, map[string]any{"ok": false, "error": "edit not implemented"}) }
func handleMessageReportV2(w http.ResponseWriter, r *http.Request)  { writeJSON(w, 501, map[string]any{"ok": false, "error": "report not implemented"}) }

// Honest receipts only (no stealth):
func handleMarkDeliveredV2(w http.ResponseWriter, r *http.Request) { writeJSON(w, 501, map[string]any{"ok": false, "error": "not implemented"}) }
func handleMarkReadV2(w http.ResponseWriter, r *http.Request)      { writeJSON(w, 501, map[string]any{"ok": false, "error": "not implemented"}) }

// -------------------------
// V2: Groups
// -------------------------

// GET /api/v2/groups/list?bot_id=xxx&limit=200
func handleGroupsListV2(w http.ResponseWriter, r *http.Request) {
	if !dbReady(w) {
		return
	}
	botID := r.URL.Query().Get("bot_id")
	_, ok := getBotByID(w, botID)
	if !ok {
		return
	}

	limit := getIntQuery(r, "limit", 200)

	q := fmt.Sprintf(`
SELECT DISTINCT chat_id
FROM %s
WHERE bot_id = ? AND (is_group = 1 OR chat_id LIKE '%%@g.us')
ORDER BY chat_id
LIMIT ?;`, historyTable)

	rows, err := historyDB.Query(q, botID, limit)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	var groups []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil && id != "" {
			groups = append(groups, id)
		}
	}

	writeJSON(w, 200, map[string]any{"ok": true, "bot_id": botID, "items": groups})
}

// GET /api/v2/group/members?bot_id=xxx&group_id=...
func handleGroupMembersV2(w http.ResponseWriter, r *http.Request) {
	botID := r.URL.Query().Get("bot_id")
	bot, ok := getBotByID(w, botID)
	if !ok {
		return
	}

	groupID := r.URL.Query().Get("group_id")
	jid, ok := parseJIDOr400(w, groupID)
	if !ok {
		return
	}

	info, err := bot.GetGroupInfo(context.Background(), jid)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	members := make([]map[string]any, 0, len(info.Participants))
	for _, p := range info.Participants {
		members = append(members, map[string]any{
			"jid":        p.JID.String(),
			"is_admin":   p.IsAdmin,
			"is_super":   p.IsSuperAdmin,
			"is_pending": p.IsPending,
		})
	}

	writeJSON(w, 200, map[string]any{
		"ok":       true,
		"bot_id":   botID,
		"group_id": jid.String(),
		"name":     info.Name,
		"members":  members,
	})
}

func handleGroupMemberAddV2(w http.ResponseWriter, r *http.Request)      { writeJSON(w, 501, map[string]any{"ok": false, "error": "add-member not implemented yet"}) }
func handleGroupMemberRemoveV2(w http.ResponseWriter, r *http.Request)   { writeJSON(w, 501, map[string]any{"ok": false, "error": "remove-member not implemented yet"}) }
func handleGroupPromoteV2(w http.ResponseWriter, r *http.Request)        { writeJSON(w, 501, map[string]any{"ok": false, "error": "promote not implemented yet"}) }
func handleGroupDemoteV2(w http.ResponseWriter, r *http.Request)         { writeJSON(w, 501, map[string]any{"ok": false, "error": "demote not implemented yet"}) }
func handleGroupSetSubjectV2(w http.ResponseWriter, r *http.Request)     { writeJSON(w, 501, map[string]any{"ok": false, "error": "set-subject not implemented"}) }
func handleGroupSetDescriptionV2(w http.ResponseWriter, r *http.Request) { writeJSON(w, 501, map[string]any{"ok": false, "error": "set-description not implemented"}) }
func handleGroupSetPhotoV2(w http.ResponseWriter, r *http.Request)       { writeJSON(w, 501, map[string]any{"ok": false, "error": "set-photo not implemented"}) }
func handleGroupInviteLinkV2(w http.ResponseWriter, r *http.Request)     { writeJSON(w, 501, map[string]any{"ok": false, "error": "invite-link not implemented"}) }
func handleGroupInviteRevokeV2(w http.ResponseWriter, r *http.Request)   { writeJSON(w, 501, map[string]any{"ok": false, "error": "invite-revoke not implemented"}) }

// -------------------------
// V2: Status / Profile / Contact / Media
// -------------------------

// These are already implemented in your main.go:
// - handleSendStatus
// - handleGetStatuses
// - handleUpdateProfile
// - handleContactInfo
// - handleSendTextV2
// - handleSendMediaV2
// - handleGetHistoryV2
//
// So we only keep optional “v2 wrappers” if you want later.
func handleStatusDeleteV2(w http.ResponseWriter, r *http.Request)  { writeJSON(w, 501, map[string]any{"ok": false, "error": "status delete not implemented"}) }
func handleStatusViewersV2(w http.ResponseWriter, r *http.Request) { writeJSON(w, 501, map[string]any{"ok": false, "error": "status viewers not implemented"}) }
func handleStatusMuteV2(w http.ResponseWriter, r *http.Request)    { writeJSON(w, 501, map[string]any{"ok": false, "error": "status mute not implemented"}) }

func handleGetProfileV2(w http.ResponseWriter, r *http.Request) {
	botID := r.URL.Query().Get("bot_id")
	bot, ok := getBotByID(w, botID)
	if !ok {
		return
	}
	writeJSON(w, 200, map[string]any{
		"ok":        true,
		"bot_id":    botID,
		"jid":       bot.Store.ID.String(),
		"is_logged": bot.IsLoggedIn(),
	})
}
func handleProfilePrivacyV2(w http.ResponseWriter, r *http.Request) { writeJSON(w, 501, map[string]any{"ok": false, "error": "profile privacy not implemented"}) }
func handleGetSettingsV2(w http.ResponseWriter, r *http.Request)    { writeJSON(w, 501, map[string]any{"ok": false, "error": "settings get not implemented"}) }
func handleUpdateSettingsV2(w http.ResponseWriter, r *http.Request) { writeJSON(w, 501, map[string]any{"ok": false, "error": "settings update not implemented"}) }

func handleContactsListV2(w http.ResponseWriter, r *http.Request) { writeJSON(w, 501, map[string]any{"ok": false, "error": "contacts list not implemented"}) }
func handleContactBlockV2(w http.ResponseWriter, r *http.Request) { writeJSON(w, 501, map[string]any{"ok": false, "error": "block not implemented"}) }
func handleContactUnblockV2(w http.ResponseWriter, r *http.Request) { writeJSON(w, 501, map[string]any{"ok": false, "error": "unblock not implemented"}) }

func handleMediaUploadV2(w http.ResponseWriter, r *http.Request)   { writeJSON(w, 501, map[string]any{"ok": false, "error": "media upload not implemented"}) }
func handleMediaDownloadV2(w http.ResponseWriter, r *http.Request) { writeJSON(w, 501, map[string]any{"ok": false, "error": "media download not implemented"}) }
func handleMediaThumbV2(w http.ResponseWriter, r *http.Request)    { writeJSON(w, 501, map[string]any{"ok": false, "error": "thumbnail not implemented"}) }

// Presence stubs
func handlePresenceSubscribeV2(w http.ResponseWriter, r *http.Request)   { writeJSON(w, 501, map[string]any{"ok": false, "error": "presence subscribe not implemented"}) }
func handlePresenceUnsubscribeV2(w http.ResponseWriter, r *http.Request) { writeJSON(w, 501, map[string]any{"ok": false, "error": "presence unsubscribe not implemented"}) }
func handlePresenceSetV2(w http.ResponseWriter, r *http.Request)         { writeJSON(w, 501, map[string]any{"ok": false, "error": "presence set not implemented"}) }

// Webhook stubs
func handleWebhookRegisterV2(w http.ResponseWriter, r *http.Request) { writeJSON(w, 501, map[string]any{"ok": false, "error": "webhook register not implemented"}) }
func handleWebhookTestV2(w http.ResponseWriter, r *http.Request)     { writeJSON(w, 200, map[string]any{"ok": true, "event": "test"}) }

// -------------------------
// OPTIONAL: Helpers used by main.go logic (if you want in same file later)
// -------------------------

// UploadToCatbox (kept here if you need media hosting)
func UploadToCatbox(data []byte, filename string) (string, error) {
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("fileToUpload", filename)
	_, _ = part.Write(data)
	_ = writer.WriteField("reqtype", "fileupload")
	_ = writer.Close()

	req, _ := http.NewRequest("POST", "https://catbox.moe/user/api.php", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return string(respBody), nil
}

// prepareAvatarJPEG (same idea as your main.go)
func prepareAvatarJPEG(input []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(input))
	if err != nil {
		return nil, fmt.Errorf("invalid image: %w", err)
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w > 640 || h > 640 {
		scaleW := float64(640) / float64(w)
		scaleH := float64(640) / float64(h)
		scale := scaleW
		if scaleH < scale {
			scale = scaleH
		}
		nw := int(float64(w) * scale)
		nh := int(float64(h) * scale)
		if nw < 1 {
			nw = 1
		}
		if nh < 1 {
			nh = 1
		}
		dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
		draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
		img = dst
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		return nil, fmt.Errorf("jpeg encode failed: %w", err)
	}
	return buf.Bytes(), nil
}

// Keep sql import reachable if moved later
var _ *sql.DB
var _ = waProto.Message{}
var _ = base64.StdEncoding