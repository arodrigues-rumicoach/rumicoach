// Package whatsapp implements messaging.Channel against Meta's WhatsApp
// Business Cloud API (Graph API). Each regional deployment runs with its own
// Meta app credentials and WhatsApp Business number.
package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rumi/rumi-be/internal/services/messaging"
	"go.uber.org/zap"
)

const graphBaseURL = "https://graph.facebook.com/v21.0"

type Client struct {
	httpClient    *http.Client
	baseURL       string
	accessToken   string
	phoneNumberID string
	logger        *zap.Logger
}

// NewClient returns nil when credentials are absent (mirrors the Twilio
// provider convention) so callers can fall back to the mock channel.
func NewClient(accessToken, phoneNumberID string, logger *zap.Logger) *Client {
	if accessToken == "" || phoneNumberID == "" {
		return nil
	}
	return &Client{
		httpClient:    &http.Client{Timeout: 30 * time.Second},
		baseURL:       graphBaseURL,
		accessToken:   accessToken,
		phoneNumberID: phoneNumberID,
		logger:        logger,
	}
}

func (c *Client) Provider() string { return "whatsapp" }

// sendResponse is the Graph API reply to POST /{phone_number_id}/messages.
type sendResponse struct {
	Messages []struct {
		ID string `json:"id"`
	} `json:"messages"`
}

func (c *Client) SendText(ctx context.Context, to messaging.Address, text string) (string, error) {
	payload := map[string]any{
		"messaging_product": "whatsapp",
		"recipient_type":    "individual",
		"to":                to.ExternalID,
		"type":              "text",
		"text":              map[string]any{"preview_url": false, "body": text},
	}
	return c.sendMessage(ctx, payload)
}

func (c *Client) SendAudio(ctx context.Context, to messaging.Address, audio []byte, mimeType string, asVoiceNote bool) (string, error) {
	mediaID, err := c.uploadMedia(ctx, audio, mimeType)
	if err != nil {
		return "", fmt.Errorf("upload audio: %w", err)
	}
	payload := map[string]any{
		"messaging_product": "whatsapp",
		"recipient_type":    "individual",
		"to":                to.ExternalID,
		"type":              "audio",
		"audio":             map[string]any{"id": mediaID},
	}
	return c.sendMessage(ctx, payload)
}

func (c *Client) SendTemplate(ctx context.Context, to messaging.Address, tmpl messaging.TemplateMessage) (string, error) {
	template := map[string]any{
		"name":     tmpl.Name,
		"language": map[string]any{"code": tmpl.Language},
	}
	if len(tmpl.BodyParams) > 0 {
		params := make([]map[string]any, 0, len(tmpl.BodyParams))
		for _, p := range tmpl.BodyParams {
			params = append(params, map[string]any{"type": "text", "text": p})
		}
		template["components"] = []map[string]any{{"type": "body", "parameters": params}}
	}
	payload := map[string]any{
		"messaging_product": "whatsapp",
		"recipient_type":    "individual",
		"to":                to.ExternalID,
		"type":              "template",
		"template":          template,
	}
	return c.sendMessage(ctx, payload)
}

func (c *Client) sendMessage(ctx context.Context, payload map[string]any) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	url := fmt.Sprintf("%s/%s/messages", c.baseURL, c.phoneNumberID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("whatsapp send: status %d: %s", resp.StatusCode, respBody)
	}
	var parsed sendResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil || len(parsed.Messages) == 0 {
		c.logger.Warn("WhatsApp send succeeded but response had no message id", zap.ByteString("body", respBody))
		return "", nil
	}
	return parsed.Messages[0].ID, nil
}
