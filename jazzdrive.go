import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

// آپ کی Railway API کا لنک
const JazzAPIUrl = "https://jazz-drive-production.up.railway.app/api"

// --- Helper Functions for Jazz Drive ---

// 1. Send OTP
func jazzGenOTP(userID, phone string) bool {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(fmt.Sprintf("%s?id=%s&gen-otp=%s", JazzAPIUrl, userID, phone))
	if err != nil {
		fmt.Println("API Error:", err)
		return false
	}
	defer resp.Body.Close()
	
	var res map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&res)
	return res["status"] == "success"
}

// 2. Verify OTP
func jazzVerifyOTP(userID, otp string) bool {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(fmt.Sprintf("%s?id=%s&verify-otp=%s", JazzAPIUrl, userID, otp))
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	var res map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&res)
	return res["status"] == "success"
}

// 3. Upload File (Streamed)
func jazzUploadFile(userID, filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	
	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return "", err
	}
	
	// Copy file data to buffer (Note: For 1GB+ files, io.Pipe is better, 
	// but for simplicity in bot logic, simple Copy is used. 
	// Railway API handles streaming, Go client needs RAM here or io.Pipe implementation)
	_, err = io.Copy(part, file)
	if err != nil {
		return "", err
	}
	writer.Close()

	req, _ := http.NewRequest("POST", fmt.Sprintf("%s?id=%s", JazzAPIUrl, userID), body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// 1 Hour Timeout for Upload
	client := &http.Client{Timeout: 3600 * time.Second} 
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var res map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&res)

	if val, ok := res["share_link"].(string); ok {
		return val, nil
	}
	return "", fmt.Errorf("Upload Failed: %v", res)
}