# -------------------
# بلڈ سٹیج (Build Stage)
# -------------------
FROM golang:1.24-alpine AS builder

# ضروری ٹولز انسٹال کریں
RUN apk add --no-cache gcc musl-dev git sqlite-dev ffmpeg-dev

WORKDIR /app

# ماڈیول فائلز کاپی کریں
COPY go.mod go.sum ./
# اگر فائلز نہ ہوں تو نیا ماڈیول بنائیں
RUN go mod init impossible-bot || true

# تمام ضروری لائبریریز ڈاؤن لوڈ کریں (بشمول MongoDB)
RUN go get go.mau.fi/whatsmeow@latest
RUN go get go.mongodb.org/mongo-driver/mongo@latest
RUN go get go.mongodb.org/mongo-driver/bson@latest
RUN go get github.com/gin-gonic/gin@latest
RUN go get github.com/lib/pq@latest
RUN go get github.com/mattn/go-sqlite3@latest
RUN go get github.com/gorilla/websocket@latest

# باقی کوڈ کاپی کریں اور ٹائیڈی کریں
COPY . .
RUN go mod tidy

# ایپ بلڈ کریں
RUN go build -o bot .

# -------------------
# فائنل سٹیج (Final Stage)
# -------------------
FROM alpine:latest

WORKDIR /app

# رن ٹائم کے لیے ضروری پیکجز
RUN apk add --no-cache ca-certificates sqlite-libs ffmpeg

# بلڈر سے فائلز کاپی کریں
COPY --from=builder /app/bot .
COPY --from=builder /app/web ./web
# اگر پکچر روٹ میں ہے تو کاپی کریں
COPY --from=builder /app/pic.png ./pic.png || true 

# پورٹ اوپن کریں
EXPOSE 8080

# بوٹ چلائیں
CMD ["./bot"]