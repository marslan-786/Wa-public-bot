FROM golang:1.24-alpine AS builder

# ضروری ٹولز: gcc اور musl-dev (sqlite اور دیگر ڈرائیورز کے لیے)، git
RUN apk add --no-cache gcc musl-dev git sqlite-dev ffmpeg-dev

WORKDIR /app
COPY . .

# فائلیں صاف کریں اور تازہ ترین لائبریریز انیشلائز کریں
RUN rm -f go.mod go.sum || true
RUN go mod init impossible-bot

# لائبریریز کے تازہ ترین ورژن حاصل کرنا
RUN go get go.mau.fi/whatsmeow@latest
RUN go get go.mongodb.org/mongo-driver/mongo@latest
RUN go get github.com/gin-gonic/gin@latest
RUN go get github.com/mattn/go-sqlite3@latest
RUN go get github.com/lib/pq@latest
RUN go mod tidy

# بوٹ بلڈ کرنا
RUN go build -o bot .

# رن سٹیج
FROM alpine:latest
RUN apk add --no-cache ca-certificates sqlite-libs ffmpeg

WORKDIR /app
COPY --from=builder /app/bot .
COPY --from=builder /app/web ./web

# ریلوے پورٹ ایکسپوز کریں
EXPOSE 8080

CMD ["./bot"]