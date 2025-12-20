FROM golang:1.24-alpine AS builder

# 1. Install Tools
RUN apk add --no-cache gcc musl-dev git sqlite-dev ffmpeg-dev

WORKDIR /app

# 2. Copy Code
COPY . .

# 3. Clean up old/conflicting files (IMPORTANT)
RUN rm -f database.go || true
RUN rm -f go.mod go.sum || true

# 4. Init Module
RUN go mod init impossible-bot

# 5. Download Libraries
RUN go get go.mau.fi/whatsmeow@latest
RUN go get go.mongodb.org/mongo-driver/mongo@latest
RUN go get go.mongodb.org/mongo-driver/bson@latest
RUN go get github.com/gin-gonic/gin@latest
RUN go get github.com/mattn/go-sqlite3@latest
RUN go get github.com/lib/pq@latest
RUN go get github.com/gorilla/websocket@latest

# 6. Tidy & Build
RUN go mod tidy
RUN go build -o bot .

# -------------------
# Final Stage
# -------------------
FROM alpine:latest

# 7. Runtime Dependencies
RUN apk add --no-cache ca-certificates sqlite-libs ffmpeg

WORKDIR /app

# 8. Copy Artifacts
COPY --from=builder /app/bot .
COPY --from=builder /app/web ./web

# چونکہ pic.png روٹ میں موجود ہے، اسے سیدھا کاپی کریں (|| true ہٹا دیا ہے)
COPY --from=builder /app/pic.png ./pic.png

# 9. Environment & Start
ENV PORT=8080
EXPOSE 8080

CMD ["./bot"]