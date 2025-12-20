# ───────────────────────────────────────────────────────────
# Stage 1: Go Builder
# ───────────────────────────────────────────────────────────
FROM golang:1.24-alpine AS go-builder
RUN apk add --no-cache gcc musl-dev git sqlite-dev
WORKDIR /app
COPY *.go ./
COPY go.mod go.sum* ./
RUN if [ ! -f go.mod ]; then \
        go mod init impossible-bot && \
        go get go.mau.fi/whatsmeow@latest && \
        go get go.mongodb.org/mongo-driver/mongo@latest && \
        go get go.mongodb.org/mongo-driver/bson@latest && \
        go get github.com/mattn/go-sqlite3@latest && \
        go get github.com/lib/pq@latest && \
        go get github.com/gorilla/websocket@latest && \
        go mod tidy; \
    fi
RUN go build -ldflags="-s -w" -o bot .

# ───────────────────────────────────────────────────────────
# Stage 2: Node.js Builder
# ───────────────────────────────────────────────────────────
FROM node:20-alpine AS node-builder
# اب یہ لائن کام کرے گی کیونکہ یہ FROM کے نیچے ہے
RUN apk add --no-cache git 

WORKDIR /app
COPY package*.json ./
COPY lid-extractor.js ./
RUN npm install --production

# ───────────────────────────────────────────────────────────
# Stage 3: Final Runtime Image
# ───────────────────────────────────────────────────────────
FROM alpine:latest
RUN apk add --no-cache \
    ca-certificates \
    sqlite-libs \
    nodejs \
    npm \
    && rm -rf /var/cache/apk/*

WORKDIR /app

# Copy files from previous stages
COPY --from=go-builder /app/bot ./bot
COPY --from=node-builder /app/node_modules ./node_modules
COPY --from=node-builder /app/lid-extractor.js ./lid-extractor.js
COPY package.json ./package.json
COPY web ./web
COPY pic.png ./pic.png

RUN mkdir -p store logs
ENV PORT=8080
ENV NODE_ENV=production
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/ || exit 1

CMD ["./bot"]
