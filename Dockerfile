# ═══════════════════════════════════════════════════════════
# 1. Stage: Go Builder (Switching to Debian for compatibility)
# ═══════════════════════════════════════════════════════════
FROM golang:1.24-bookworm AS go-builder

# ڈائبیئن کے لیے ضروری ٹولز
RUN apt-get update && apt-get install -y \
    gcc \
    libc6-dev \
    git \
    libsqlite3-dev \
    ffmpeg \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY . .
RUN rm -f go.mod go.sum || true

RUN go mod init impossible-bot && \
    go get go.mau.fi/whatsmeow@latest && \
    go get go.mongodb.org/mongo-driver/mongo@latest && \
    go get go.mongodb.org/mongo-driver/bson@latest && \
    go get github.com/redis/go-redis/v9@latest && \
    go get github.com/gin-gonic/gin@latest && \
    go get github.com/mattn/go-sqlite3@latest && \
    go get github.com/lib/pq@latest && \
    go get github.com/gorilla/websocket@latest && \
    go get google.golang.org/protobuf/proto@latest && \
    go get github.com/showwin/speedtest-go && \
    go mod tidy

# 🚀 بوٹ کو بلڈ کریں (CGO انیبل رکھا ہے کیونکہ sqlite ضروری ہے)
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o bot .

# ═══════════════════════════════════════════════════════════
# 2. Stage: Node.js Builder
# ═══════════════════════════════════════════════════════════
FROM node:20-bookworm-slim AS node-builder
WORKDIR /app
COPY package*.json ./
COPY lid-extractor.js ./
RUN npm install --production

# ═══════════════════════════════════════════════════════════
# 3. Stage: Final Runtime (The 32GB RAM Monster)
# ═══════════════════════════════════════════════════════════
FROM python:3.12-slim-bookworm

# ضروری سسٹم لائبریریز
RUN apt-get update && apt-get install -y \
    ffmpeg \
    curl \
    sqlite3 \
    libsqlite3-0 \
    nodejs \
    npm \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# yt-dlp انسٹالیشن
RUN curl -L https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp -o /usr/local/bin/yt-dlp \
    && chmod a+rx /usr/local/bin/yt-dlp

# rembg انسٹالیشن
RUN pip3 install --no-cache-dir rembg[cli]

WORKDIR /app

# بلڈرز سے بوٹ اور نوڈ ماڈیولز اٹھائیں
COPY --from=go-builder /app/bot ./bot
COPY --from=node-builder /app/node_modules ./node_modules
COPY --from=node-builder /app/lid-extractor.js ./lid-extractor.js
COPY --from=node-builder /app/package.json ./package.json

COPY web ./web
COPY pic.png ./pic.png

RUN mkdir -p store logs

# 🎯 انوائرمنٹ سیٹنگز
ENV PORT=8080
ENV NODE_ENV=production
ENV U2NET_HOME=/app/store/.u2net 

EXPOSE 8080

# ✅ کمانڈ کو تھوڑا بدل دیا ہے تاکہ ڈائریکٹ ایگزیکیوٹ ہو
CMD ["/app/bot"]