package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rumi/rumi-be/internal/services/messaging"
	"go.uber.org/zap"
)

const telegramBaseURL = "https://api.telegram.org/bot"

type Client struct {
	httpClient *http.Client
	baseURL    string
	token      string
	logger     *zap.Logger
}

func NewClient(token string, logger *zap.Logger) *Client {
	if token == "" {
		return nil
	}
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    telegramBaseURL + token,
		token:      token,
		logger:     logger,
	}
}

func (c *Client) Provider() string { return "telegram" }

type apiResponse struct {
	Ok          bool            `json:"ok"`
	ErrorCode   int             `json:"error_code,omitempty"`
	Description string          `json:"description,omitempty"`
	Result      json.RawMessage `json:"result,omitempty"`
}

type messageResult struct {
	MessageID int `json:"message_id"`
}

func (c *Client) SendText(ctx context.Context, to messaging.Address, text string) (string, error) {
	payload := map[string]any{
		"chat_id":    to.ExternalID,
		"text":       text,
		"parse_mode": "HTML",
	}
	return c.sendMessage(ctx, "sendMessage", payload)
}

func (c *Client) SendAudio(ctx context.Context, to messaging.Address, audio []byte, mimeType string, asVoiceNote bool) (string, error) {
	// The API method and the multipart FIELD NAME are different things: sendVoice reads
	// the file from a field called "voice", sendAudio from "audio". Naming the field
	// after the method made Telegram answer "there is no voice in the request" and every
	// voice reply silently fell back to text.
	method, field, filename := "sendAudio", "audio", "audio.ogg"
	if asVoiceNote {
		// Telegram only accepts OGG/Opus for voice notes.
		method, field, filename = "sendVoice", "voice", "voice.ogg"
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	_ = writer.WriteField("chat_id", to.ExternalID)

	part, err := writer.CreateFormFile(field, filename)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(part, bytes.NewReader(audio)); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/%s", c.baseURL, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	return c.executeRequest(req)
}

func (c *Client) SendTemplate(ctx context.Context, to messaging.Address, tmpl messaging.TemplateMessage) (string, error) {
	// Telegram does not use Meta pre-approved templates.
	// Map known template names to human-readable text and clean up fallbacks
	// to prevent raw identifiers like [rumi_reengage] from reaching users.
	var text string
	switch tmpl.Name {
	case "rumi_reengage":
		text = "Olá! Já não conversamos há algum tempo. Quando quiseres dar continuidade aos teus planos, estou por aqui."
	default:
		if len(tmpl.BodyParams) > 0 {
			text = strings.Join(tmpl.BodyParams, "\n")
		} else {
			text = "Olá! Estou por aqui se precisares de conversar ou planear os teus próximos passos."
		}
	}
	return c.SendText(ctx, to, text)
}

func (c *Client) DownloadMedia(ctx context.Context, mediaID string) ([]byte, string, error) {
	url := fmt.Sprintf("%s/getFile?file_id=%s", c.baseURL, mediaID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("telegram getFile: status %d", resp.StatusCode)
	}

	var parsed apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, "", err
	}
	if !parsed.Ok {
		return nil, "", fmt.Errorf("telegram getFile error: %d %s", parsed.ErrorCode, parsed.Description)
	}

	var fileResult struct {
		FilePath string `json:"file_path"`
	}
	if err := json.Unmarshal(parsed.Result, &fileResult); err != nil {
		return nil, "", err
	}

	downloadURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", c.token, fileResult.FilePath)
	dlReq, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, "", err
	}

	dlResp, err := c.httpClient.Do(dlReq)
	if err != nil {
		return nil, "", err
	}
	defer dlResp.Body.Close()

	if dlResp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("telegram download: status %d", dlResp.StatusCode)
	}

	data, err := io.ReadAll(dlResp.Body)
	if err != nil {
		return nil, "", err
	}

	// Telegram's file CDN serves voice notes as application/octet-stream, which the
	// transcription API rejects outright ("Unsupported MIME type"). The file path it
	// gave us carries the real format, so resolve the type from that and only trust the
	// header when it is actually specific.
	contentType := dlResp.Header.Get("Content-Type")
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = mimeFromPath(fileResult.FilePath)
	}

	return data, contentType, nil
}

// mimeFromPath maps a Telegram file_path (e.g. "voice/file_12.oga") to a MIME type the
// transcription API accepts. Voice notes are always OGG/Opus, which is also the safest
// fallback for an unknown audio extension.
func mimeFromPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".oga", ".ogg", ".opus":
		return "audio/ogg"
	case ".mp3":
		return "audio/mpeg"
	case ".m4a", ".mp4":
		return "audio/mp4"
	case ".wav":
		return "audio/wav"
	case ".aac":
		return "audio/aac"
	case ".flac":
		return "audio/flac"
	case ".webm":
		return "audio/webm"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	default:
		// Voice notes are the only media this channel transcribes, and Telegram always
		// encodes them as OGG/Opus — a better guess than octet-stream, which is
		// guaranteed to fail.
		return "audio/ogg"
	}
}

func (c *Client) sendMessage(ctx context.Context, method string, payload map[string]any) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	url := fmt.Sprintf("%s/%s", c.baseURL, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	return c.executeRequest(req)
}

func (c *Client) executeRequest(req *http.Request) (string, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("telegram send: status %d: %s", resp.StatusCode, respBody)
	}

	var parsed apiResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		c.logger.Warn("Telegram send succeeded but response unparseable", zap.ByteString("body", respBody))
		return "", nil
	}
	if !parsed.Ok {
		return "", fmt.Errorf("telegram api error: %d %s", parsed.ErrorCode, parsed.Description)
	}

	var msgResult messageResult
	if err := json.Unmarshal(parsed.Result, &msgResult); err != nil || msgResult.MessageID == 0 {
		c.logger.Warn("Telegram send succeeded but response had no message id", zap.ByteString("body", respBody))
		return "", nil
	}

	return strconv.Itoa(msgResult.MessageID), nil
}
