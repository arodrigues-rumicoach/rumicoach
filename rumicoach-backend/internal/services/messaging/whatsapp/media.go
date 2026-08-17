package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
)

// uploadMedia uploads a media blob and returns the Graph API media id.
func (c *Client) uploadMedia(ctx context.Context, data []byte, mimeType string) (string, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	if err := writer.WriteField("messaging_product", "whatsapp"); err != nil {
		return "", err
	}
	if err := writer.WriteField("type", mimeType); err != nil {
		return "", err
	}
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="file"; filename="audio"`)
	header.Set("Content-Type", mimeType)
	part, err := writer.CreatePart(header)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(data); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/%s/media", c.baseURL, c.phoneNumberID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("whatsapp media upload: status %d: %s", resp.StatusCode, respBody)
	}
	var parsed struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("whatsapp media upload: parse response: %w", err)
	}
	if parsed.ID == "" {
		return "", fmt.Errorf("whatsapp media upload: empty media id in response: %s", respBody)
	}
	return parsed.ID, nil
}

// DownloadMedia resolves a media id to its short-lived URL and fetches the
// bytes (both steps require the bearer token).
func (c *Client) DownloadMedia(ctx context.Context, mediaID string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/%s", c.baseURL, mediaID), nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("whatsapp media lookup: status %d: %s", resp.StatusCode, respBody)
	}
	var meta struct {
		URL      string `json:"url"`
		MimeType string `json:"mime_type"`
	}
	if err := json.Unmarshal(respBody, &meta); err != nil {
		return nil, "", fmt.Errorf("whatsapp media lookup: parse response: %w", err)
	}
	if meta.URL == "" {
		return nil, "", fmt.Errorf("whatsapp media lookup: no url for media %s", mediaID)
	}

	dlReq, err := http.NewRequestWithContext(ctx, http.MethodGet, meta.URL, nil)
	if err != nil {
		return nil, "", err
	}
	dlReq.Header.Set("Authorization", "Bearer "+c.accessToken)
	dlResp, err := c.httpClient.Do(dlReq)
	if err != nil {
		return nil, "", err
	}
	defer dlResp.Body.Close()
	if dlResp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("whatsapp media download: status %d", dlResp.StatusCode)
	}
	// Voice notes are small (~1 KB/s); 32 MB guards against abuse.
	data, err := io.ReadAll(io.LimitReader(dlResp.Body, 32<<20))
	if err != nil {
		return nil, "", err
	}
	return data, meta.MimeType, nil
}
