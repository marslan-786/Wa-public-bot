package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func saveGroupSettings(s *GroupSettings) {
	// 1. پہلے میموری (RAM) میں اپڈیٹ کریں (فاسٹ ایکسیس کے لیے)
	cacheMutex.Lock()
	groupCache[s.ChatID] = s
	cacheMutex.Unlock()

	// 2. اب Redis میں ہمیشہ کے لیے سیو کریں
	if rdb != nil {
		// ڈیٹا کو JSON میں تبدیل کریں
		jsonData, err := json.Marshal(s)
		if err == nil {
			// Redis Key: "group_settings:12036..."
			key := "group_settings:" + s.ChatID
			
			// Redis میں سیو کریں (0 کا مطلب ہے کبھی ایکسپائر نہ ہو)
			err := rdb.Set(ctx, key, jsonData, 0).Err()
			if err != nil {
				fmt.Printf("⚠️ [REDIS ERROR] Failed to save settings for %s: %v\n", s.ChatID, err)
			} else {
				// fmt.Println("✅ Settings saved to Redis") // (Optional Log)
			}
		}
	}
}


func getGroupSettings(chatID string) *GroupSettings {
	// 1. پہلے میموری (RAM) چیک کریں
	cacheMutex.RLock()
	s, exists := groupCache[chatID]
	cacheMutex.RUnlock()

	if exists {
		return s
	}

	// 2. اگر میموری میں نہیں ہے، تو Redis چیک کریں
	if rdb != nil {
		key := "group_settings:" + chatID
		val, err := rdb.Get(ctx, key).Result()
		
		if err == nil {
			// Redis سے ڈیٹا مل گیا! اب اسے واپس Struct میں ڈالیں
			var loadedSettings GroupSettings
			err := json.Unmarshal([]byte(val), &loadedSettings)
			if err == nil {
				// میموری میں بھی رکھ لیں تاکہ اگلی بار Redis کو کال نہ کرنی پڑے
				cacheMutex.Lock()
				groupCache[chatID] = &loadedSettings
				cacheMutex.Unlock()
				
				return &loadedSettings
			}
		}
	}

	// 3. اگر Redis میں بھی نہیں ہے، تو ڈیفالٹ سیٹنگز بنا کر دیں
	// (پہلی بار جب گروپ میں بوٹ آئے گا)
	newSettings := &GroupSettings{
		ChatID:         chatID,
		Mode:           "public", // ڈیفالٹ موڈ
		Antilink:       false,
		AntilinkAdmin:  true,     // ڈیفالٹ: ایڈمن لنک بھیج سکے
		AntilinkAction: "delete", // ڈیفالٹ ایکشن
		Welcome:        false,
		Warnings:       make(map[string]int),
	}

	return newSettings
}

// ==================== سیٹنگز سسٹم ====================
func toggleAlwaysOnline(client *whatsmeow.Client, v *events.Message) {
	if !isOwner(client, v.Info.Sender) {
		msg := `╔════════════════╗
║ ❌ ACCESS DENIED
╠════════════════╣
║ 🔒 Owner Only
╚════════════════╝`
		replyMessage(client, v, msg)
		return
	}

	status := "OFF 🔴"
	statusText := "Disabled"
	dataMutex.Lock()
	data.AlwaysOnline = !data.AlwaysOnline
	if data.AlwaysOnline {
		client.SendPresence(context.Background(), types.PresenceAvailable)
		status = "ON 🟢"
		statusText = "Enabled"
	} else {
		client.SendPresence(context.Background(), types.PresenceUnavailable)
	}
	dataMutex.Unlock()

	msg := fmt.Sprintf(`╔════════════════╗
║ ⚙️ ALWAYS ONLINE
╠════════════════╣
║ 📊 Status: %s
║ 🔄 State: %s
║ ✅ Updated
╚════════════════╝`, status, statusText)

	replyMessage(client, v, msg)
}

func toggleAutoRead(client *whatsmeow.Client, v *events.Message) {
	if !isOwner(client, v.Info.Sender) {
		msg := `╔════════════════╗
║ ❌ ACCESS DENIED
╠════════════════╣
║ 🔒 Owner Only
╚════════════════╝`
		replyMessage(client, v, msg)
		return
	}

	status := "OFF 🔴"
	statusText := "Disabled"
	dataMutex.Lock()
	data.AutoRead = !data.AutoRead
	if data.AutoRead {
		status = "ON 🟢"
		statusText = "Enabled"
	}
	dataMutex.Unlock()

	msg := fmt.Sprintf(`╔════════════════╗
║ ⚙️ AUTO READ
╠════════════════╣
║ 📊 Status: %s
║ 🔄 State: %s
║ ✅ Updated
╚════════════════╝`, status, statusText)

	replyMessage(client, v, msg)
}

func toggleAutoReact(client *whatsmeow.Client, v *events.Message) {
	// 1. Permission Check
	if !isOwner(client, v.Info.Sender) {
		msg := `╔════════════════╗
║ ❌ ACCESS DENIED
╠════════════════╣
║ 🔒 Owner Only
╚════════════════╝`
		replyMessage(client, v, msg)
		return
	}

	// 2. Parse Arguments
	// میسج سے ٹیکسٹ نکال کر چیک کریں کہ آگے "on" لکھا ہے یا "off"
	body := strings.TrimSpace(getText(v.Message))
	parts := strings.Fields(body)

	dataMutex.Lock()
	defer dataMutex.Unlock()

	// 3. اگر صرف کمانڈ ہے (.autoreact) تو اسٹیٹس دکھائیں
	if len(parts) == 1 {
		statusIcon := "🔴"
		statusText := "Disabled"
		if data.AutoReact {
			statusIcon = "🟢"
			statusText = "Enabled"
		}

		msg := fmt.Sprintf(`╔════════════════╗
║ ⚙️ AUTO REACT INFO
╠════════════════╣
║ 📊 Status: %s
║ 📝 State: %s
╚════════════════╝`, statusIcon, statusText)
		replyMessage(client, v, msg)
		return
	}

	// 4. ON / OFF Logic
	action := strings.ToLower(parts[1])

	if action == "on" || action == "enable" {
		if data.AutoReact {
			// اگر پہلے سے آن ہے
			msg := `╔════════════════╗
║ ⚠️ ALREADY ACTIVE
╠════════════════╣
║ Auto React is
║ already ON 🟢
╚════════════════╝`
			replyMessage(client, v, msg)
		} else {
			// اب آن کریں
			data.AutoReact = true
			msg := `╔════════════════╗
║ ✅ SUCCESS
╠════════════════╣
║ Auto React has
║ been Enabled 🟢
╚════════════════╝`
			replyMessage(client, v, msg)
		}
	} else if action == "off" || action == "disable" {
		if !data.AutoReact {
			// اگر پہلے سے آف ہے
			msg := `╔════════════════╗
║ ⚠️ ALREADY OFF
╠════════════════╣
║ Auto React is
║ already OFF 🔴
╚════════════════╝`
			replyMessage(client, v, msg)
		} else {
			// اب آف کریں
			data.AutoReact = false
			msg := `╔════════════════╗
║ 🛑 STOPPED
╠════════════════╣
║ Auto React has
║ been Disabled 🔴
╚════════════════╝`
			replyMessage(client, v, msg)
		}
	} else {
		// غلط کمانڈ
		replyMessage(client, v, "⚠️ Usage: .autoreact on | off")
	}
}

// ✅ گلوبل سیٹنگز سیو کرنے کا ہیلپر فنکشن
func saveGlobalSettings() {
	if rdb != nil {
		jsonBytes, _ := json.Marshal(data)
		rdb.Set(ctx, "bot_global_settings", jsonBytes, 0)
	}
}

func toggleAutoStatus(client *whatsmeow.Client, v *events.Message) {
	if !isOwner(client, v.Info.Sender) {
		replyMessage(client, v, "❌ Owner Only!")
		return
	}

	// 1. آرگیومنٹس پارس کریں
	body := strings.TrimSpace(getText(v.Message))
	parts := strings.Fields(body)

	dataMutex.Lock()
	defer dataMutex.Unlock()

	// 2. اگر صرف سٹیٹس چیک کرنا ہو
	if len(parts) == 1 {
		status := "OFF 🔴"
		if data.AutoStatus { status = "ON 🟢" }
		replyMessage(client, v, fmt.Sprintf("📊 *Auto Status:* %s", status))
		return
	}

	// 3. On/Off لاجک
	arg := strings.ToLower(parts[1])
	if arg == "on" || arg == "enable" {
		data.AutoStatus = true
	} else if arg == "off" || arg == "disable" {
		data.AutoStatus = false
	} else {
		replyMessage(client, v, "⚠️ Usage: .autostatus on | off")
		return
	}

	// 4. ✅ Redis میں سیو کریں (تاکہ ری سٹارٹ پر یاد رہے)
	saveGlobalSettings()

	state := "Disabled"
	icon := "🔴"
	if data.AutoStatus {
		state = "Enabled"
		icon = "🟢"
	}

	msg := fmt.Sprintf(`╔════════════════╗
║ ⚙️ AUTO STATUS
╠════════════════╣
║ 📊 Status: %s
║ 🔄 State: %s
║ ✅ Saved to DB
╚════════════════╝`, icon, state)
	replyMessage(client, v, msg)
}

func toggleStatusReact(client *whatsmeow.Client, v *events.Message) {
	if !isOwner(client, v.Info.Sender) {
		replyMessage(client, v, "❌ Owner Only!")
		return
	}

	body := strings.TrimSpace(getText(v.Message))
	parts := strings.Fields(body)

	dataMutex.Lock()
	defer dataMutex.Unlock()

	if len(parts) == 1 {
		status := "OFF 🔴"
		if data.StatusReact { status = "ON 🟢" }
		replyMessage(client, v, fmt.Sprintf("📊 *Status React:* %s", status))
		return
	}

	arg := strings.ToLower(parts[1])
	if arg == "on" || arg == "enable" {
		data.StatusReact = true
	} else if arg == "off" || arg == "disable" {
		data.StatusReact = false
	} else {
		replyMessage(client, v, "⚠️ Usage: .statusreact on | off")
		return
	}

	// ✅ Redis Save
	saveGlobalSettings()

	state := "Disabled"
	icon := "🔴"
	if data.StatusReact {
		state = "Enabled"
		icon = "🟢"
	}

	msg := fmt.Sprintf(`╔════════════════╗
║ ⚙️ STATUS REACT
╠════════════════╣
║ 📊 Status: %s
║ 🔄 State: %s
║ ✅ Saved to DB
╚════════════════╝`, icon, state)
	replyMessage(client, v, msg)
}

func handleAddStatus(client *whatsmeow.Client, v *events.Message, args []string) {
	if !isOwner(client, v.Info.Sender) {
		msg := `╔════════════════╗
║ ❌ ACCESS DENIED
╠════════════════╣
║ 🔒 Owner Only
╚════════════════╝`
		replyMessage(client, v, msg)
		return
	}

	if len(args) < 1 {
		msg := `╔════════════════╗
║ ⚠️ INVALID FORMAT
╠════════════════╣
║ 📝 .addstatus <num>
║ 💡 .addstatus 923xx
╚════════════════╝`
		replyMessage(client, v, msg)
		return
	}

	num := args[0]
	dataMutex.Lock()
	data.StatusTargets = append(data.StatusTargets, num)
	dataMutex.Unlock()

	msg := fmt.Sprintf(`╔════════════════╗
║ ✅ TARGET ADDED
╠════════════════╣
║ 📱 %s
║ 📊 Total: %d
╚════════════════╝`, num, len(data.StatusTargets))

	replyMessage(client, v, msg)
}

func handleDelStatus(client *whatsmeow.Client, v *events.Message, args []string) {
	if !isOwner(client, v.Info.Sender) {
		msg := `╔════════════════╗
║ ❌ ACCESS DENIED
╠════════════════╣
║ 🔒 Owner Only
╚════════════════╝`
		replyMessage(client, v, msg)
		return
	}

	if len(args) < 1 {
		msg := `╔════════════════╗
║ ⚠️ INVALID FORMAT
╠════════════════╣
║ 📝 .delstatus <num>
║ 💡 .delstatus 923xx
╚════════════════╝`
		replyMessage(client, v, msg)
		return
	}

	num := args[0]
	dataMutex.Lock()
	newList := []string{}
	found := false
	for _, n := range data.StatusTargets {
		if n != num {
			newList = append(newList, n)
		} else {
			found = true
		}
	}
	data.StatusTargets = newList
	dataMutex.Unlock()

	if found {
		msg := fmt.Sprintf(`╔════════════════╗
║ ✅ TARGET REMOVED
╠════════════════╣
║ 📱 %s
║ 📊 Remaining: %d
╚════════════════╝`, num, len(data.StatusTargets))
		replyMessage(client, v, msg)
	} else {
		msg := `╔════════════════╗
║ ❌ NOT FOUND
╠════════════════╣
║ Number not in list
╚════════════════╝`
		replyMessage(client, v, msg)
	}
}

func handleListStatus(client *whatsmeow.Client, v *events.Message) {
	if !isOwner(client, v.Info.Sender) {
		return
	}

	dataMutex.RLock()
	targets := data.StatusTargets
	dataMutex.RUnlock()

	if len(targets) == 0 {
		msg := `╔════════════════╗
║ 📭 NO TARGETS
╠════════════════╣
║ Use .addstatus
╚════════════════╝`
		replyMessage(client, v, msg)
		return
	}

	msg := "╔════════════════╗\n"
	msg += "║ 📜 STATUS TARGETS\n"
	msg += "╠════════════════╣\n"
	for i, t := range targets {
		msg += fmt.Sprintf("║ %d. %s\n", i+1, t)
	}
	msg += fmt.Sprintf("║ 📊 Total: %d\n", len(targets))
	msg += "╚════════════════╝"

	replyMessage(client, v, msg)
}

func handleSetPrefix(client *whatsmeow.Client, v *events.Message, args []string) {
	if !isOwner(client, v.Info.Sender) {
		msg := `╔════════════════╗
║ ❌ ACCESS DENIED
╠════════════════╣
║ 🔒 Owner Only
╚════════════════╝`
		replyMessage(client, v, msg)
		return
	}

	if len(args) < 1 {
		msg := `╔════════════════╗
║ ⚠️ INVALID FORMAT
╠════════════════╣
║ 📝 .setprefix <sym>
║ 💡 .setprefix .
║ 💡 .setprefix !
╚════════════════╝`
		replyMessage(client, v, msg)
		return
	}

	newPrefix := args[0]
	dataMutex.Lock()
	data.Prefix = newPrefix
	dataMutex.Unlock()

	msg := fmt.Sprintf(`╔════════════════╗
║ ✅ PREFIX UPDATED
╠════════════════╣
║ 🔧 New: %s
║ 💡 Ex: %smenu
╚════════════════╝`, newPrefix, newPrefix)

	replyMessage(client, v, msg)
}

func handleMode(client *whatsmeow.Client, v *events.Message, args []string) {
	// Owner check
	if !isOwner(client, v.Info.Sender) {
		msg := `╔════════════════╗
║ ❌ ACCESS DENIED
╠════════════════╣
║ 🔒 Owner Only
╚════════════════╝`
		replyMessage(client, v, msg)
		return
	}

	// Private chat - show all groups with their modes
	if !v.Info.IsGroup {
		if len(args) < 1 {
			msg := `╔════════════════╗
║ ⚙️ GROUP MODE
╠════════════════╣
║ 1️⃣ public - All
║ 2️⃣ private - Off
║ 3️⃣ admin - Admin
║ 📝 .mode <type>
║ 💡 Use in group
║    to change mode
╚════════════════╝`
			replyMessage(client, v, msg)
			return
		}
	}

	// Group chat - change mode
	if v.Info.IsGroup {
		if len(args) < 1 {
			msg := `╔════════════════╗
║ ⚙️ GROUP MODE
╠════════════════╣
║ 1️⃣ public - All
║ 2️⃣ private - Off
║ 3️⃣ admin - Admin
║ 📝 .mode <type>
╚════════════════╝`
			replyMessage(client, v, msg)
			return
		}

		mode := strings.ToLower(args[0])
		if mode != "public" && mode != "private" && mode != "admin" {
			msg := `╔════════════════╗
║ ❌ INVALID MODE
╠════════════════╣
║ Use: public/
║ private/admin
╚════════════════╝`
			replyMessage(client, v, msg)
			return
		}

		s := getGroupSettings(v.Info.Chat.String())
		s.Mode = mode
		saveGroupSettings(s)

		var modeDesc string
		switch mode {
		case "public":
			modeDesc = "Everyone"
		case "private":
			modeDesc = "Disabled"
		case "admin":
			modeDesc = "Admin only"
		}

		msg := fmt.Sprintf(`╔════════════════╗
║ ✅ MODE CHANGED
╠════════════════╣
║ 🛡️ %s
║ 📝 %s
║ ✅ Updated
╚════════════════╝`, strings.ToUpper(mode), modeDesc)

		replyMessage(client, v, msg)
	}
}

func handleReadAllStatus(client *whatsmeow.Client, v *events.Message) {
	if !isOwner(client, v.Info.Sender) {
		return
	}

	client.MarkRead(context.Background(), []types.MessageID{v.Info.ID}, time.Now(), types.NewJID("status@broadcast", types.DefaultUserServer), v.Info.Sender, types.ReceiptTypeRead)

	msg := `╔════════════════╗
║ ✅ STATUSES READ
╠════════════════╣
║ All marked read
╚════════════════╝`

	replyMessage(client, v, msg)
}