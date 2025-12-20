# 🚂 Railway Deployment - LID System Setup Guide

## 📋 Complete Setup for Automatic LID Extraction

یہ guide **Railway deployment** کے لیے ہے جہاں سب کچھ **automatically** ہوگا۔

---

## 📦 Required Files

اپنے project میں یہ files add کریں:

```
your-project/
├── main.go                 # ✅ Updated (provided)
├── lid_system.go          # 🆕 NEW (provided)
├── commands.go            # ✅ Updated (provided)
├── lid-extractor.js       # 🆕 NEW (provided)
├── package.json           # ✅ Your existing one
├── go.mod                 # ✅ Your existing one
├── web/
│   └── index.html        # ✅ Your existing file
└── pic.png               # ✅ Your existing file
```

---

## 🔧 Step-by-Step Setup

### Step 1: Copy Files

```bash
# اپنے project folder میں
cp lid_system.go ./
cp lid-extractor.js ./
# main.go اور commands.go کو replace کریں
```

### Step 2: Install Node.js Dependencies

آپ کی `package.json` میں پہلے سے **Baileys 6.7.4** موجود ہے، بس check کریں:

```json
{
  "dependencies": {
    "@whiskeysockets/baileys": "^6.7.4",
    "@hapi/boom": "^10.0.1",
    "pino": "^9.3.0"
  }
}
```

✅ یہ dependencies پہلے سے ہیں، کوئی نیا install کرنے کی ضرورت نہیں۔

### Step 3: Railway Configuration

Railway پر deploy کرتے وقت **build command** set کریں:

```bash
# Build command
npm install && go build -o bot .

# Start command
./bot
```

یا `Procfile` بنائیں:

```
web: ./bot
```

### Step 4: Environment Variables (Optional)

Railway dashboard میں environment variables:

```
PORT=8080
DATABASE_URL=your_postgres_url (if using)
```

---

## 🚀 How It Works

### Automatic Flow:

```
1. Bot starts
   ↓
2. main.go calls InitLIDSystem()
   ↓
3. InitLIDSystem() runs:
   - Checks MongoDB for existing LIDs
   - Runs Node.js extractor (child process)
   - Loads extracted data
   - Syncs to MongoDB
   ↓
4. System ready!
```

### On New Pairing:

```
1. User pairs via /api/pair
   ↓
2. Pairing succeeds
   ↓
3. OnNewPairing(client) called
   ↓
4. Node.js extractor runs again
   ↓
5. New LID extracted & saved
   ↓
6. Ready to use!
```

---

## 📊 Console Output (Expected)

جب bot start ہو تو console میں یہ دیکھیں گے:

```
🚀 IMPOSSIBLE BOT | START
✅ MongoDB connected

╔═══════════════════════════════════════╗
║   🔐 LID SYSTEM INITIALIZING         ║
╚═══════════════════════════════════════╝

📊 Checking MongoDB for existing LIDs...
✅ Loaded 2 LID(s) from MongoDB

🔍 Running LID extractor...

╔═══════════════════════════════════════╗
║   🔍 LID EXTRACTION STARTED          ║
╚═══════════════════════════════════════╝

📁 Found 2 file(s) in store directory
━━━ Scanning: device-1 ━━━
📞 Phone: 923001234567
✅ LID found (me.lid): 123456789012345
📱 Platform: Chrome

✅ Extracted LID for: 923001234567

📊 Total LIDs extracted: 2
✅ Results saved to: ./lid_data.json
⏱️ Extraction completed in 1.23s
✅ SUCCESS: LIDs extracted and saved

📂 Loading LID data from file...
✅ Loaded 2 LID(s) from cache

📊 Registered Bot LIDs:
   📱 923001234567 → 🆔 123456789012345
   📱 923009876543 → 🆔 987654321098765

💾 Syncing to MongoDB...
✅ Saved to MongoDB: 923001234567 → 123456789012345
✅ Saved to MongoDB: 923009876543 → 987654321098765

╔═══════════════════════════════════════╗
║   ✅ LID SYSTEM READY (2 bots)       ║
╚═══════════════════════════════════════╝

🌐 Web Server running on port 8080
```

---

## 🔐 Testing Owner Verification

### Test 1: Check Owner Status

WhatsApp میں bot کو message:

```
!owner
```

**Response:**
```
╔════════════════════════════╗
║ 👑 OWNER STATUS
╠════════════════════════════╣
║ 📱 Bot: 923001234567
║ 🆔 LID: 123456789012345
║ 👤 You: 123456789012345
║ 
║ ✅ YOU are Owner
╠════════════════════════════╣
║ 🔐 LID-Based Verification
╚════════════════════════════╝
```

### Test 2: List All Bots

```
!listbots
```

**Response (Owner Only):**
```
╔════════════════════════════╗
║ 📊 REGISTERED BOTS
╠════════════════════════════╣
║ 1. 923001234567
║    🆔 123456789012345
║
║ 2. 923009876543
║    🆔 987654321098765
║
╚════════════════════════════╝
```

---

## 🗂️ Files Generated

Bot runtime میں یہ files automatically بنے گی:

```
lid_data.json          # Extracted LID data
lid_extractor.log      # Extraction logs
impossible.db          # SQLite session storage
store/                 # whatsmeow session files
```

---

## 🔧 MongoDB Structure

MongoDB میں data ایسے save ہوگا:

```json
{
  "_id": "...",
  "phone": "923001234567",
  "lid": "123456789012345",
  "platform": "Chrome",
  "sessionId": "device-1",
  "extractedAt": "2025-12-20T10:30:00Z",
  "lastUpdated": "2025-12-20T11:00:00Z"
}
```

---

## 🚨 Troubleshooting

### Problem: "Node.js not found"

**Solution:**
Railway پر Node.js automatically available ہے۔ اگر error آئے تو:

```bash
# Railway buildpack میں یہ add کریں
heroku/nodejs
heroku/go
```

### Problem: "No LIDs found"

**Solution:**
- First run پر یہ normal ہے
- پہلے device pair کریں: `/api/pair`
- Pair ہونے کے بعد LID خود extract ہو جائے گی

### Problem: "Extractor timeout"

**Solution:**
`lid-extractor.js` میں timeout بڑھائیں:

```javascript
AUTO_EXIT_TIMEOUT: 60000, // 60 seconds
```

### Problem: "Cannot read store directory"

**Solution:**
`lid-extractor.js` میں sessions path check کریں:

```javascript
SESSIONS_DIR: './store',  // یا './sessions'
```

---

## 📱 API Endpoints

### 1. Pair New Device

```bash
curl -X POST http://your-app.railway.app/api/pair \
  -H "Content-Type: application/json" \
  -d '{"number":"923001234567"}'
```

**Response:**
```json
{
  "success": true,
  "code": "ABC-DEF-GHI"
}
```

### 2. Check Connection

```bash
curl http://your-app.railway.app/ws
```

---

## 🎯 Command List

| Command | Description | Owner Only |
|---------|-------------|------------|
| `!owner` | Check owner status | ❌ |
| `!listbots` | List all bots with LIDs | ✅ |
| `!ping` | Check bot speed | ❌ |
| `!id` | Get chat/user IDs | ❌ |
| `!menu` | Show all commands | ❌ |
| `!mode` | Change bot mode | ✅ |

---

## 🔄 Update Process

اگر code update کرنا ہو:

```bash
# Railway automatically redeploys on push
git add .
git commit -m "Updated LID system"
git push railway main
```

---

## 📊 Monitoring

### Check Logs:

Railway dashboard میں:
```
Deployments > Latest > View Logs
```

### Check Database:

MongoDB compass یا CLI سے:
```bash
mongosh "mongodb://mongo:PASSWORD@HOST:PORT"
use impossible_db
db.bot_data.find()
```

---

## ✅ Deployment Checklist

- [ ] `lid_system.go` added
- [ ] `lid-extractor.js` added
- [ ] `main.go` updated
- [ ] `commands.go` updated
- [ ] `package.json` has Baileys 6.7.4
- [ ] MongoDB connection string set
- [ ] Pushed to Railway
- [ ] Bot started successfully
- [ ] Paired at least one device
- [ ] Tested `!owner` command
- [ ] Verified LID in MongoDB

---

## 🎉 Success Indicators

✅ Console shows: "LID SYSTEM READY"
✅ `!owner` command works correctly
✅ `!listbots` shows registered devices
✅ MongoDB has LID entries
✅ New pairings auto-extract LID

---

## 🆘 Support

اگر کوئی issue ہو تو check کریں:

1. Console logs (`lid_extractor.log`)
2. MongoDB entries
3. `lid_data.json` file exists
4. Node.js version (should be 16+)
5. Go version (should be 1.19+)

---

یہ system **fully automatic** ہے! Railway پر deploy کرنے کے بعد سب کچھ خود ہو جائے گا۔ 🚀