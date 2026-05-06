// Package api provides an HTTP client for the fmsg-webapi service.
package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
)

// Client is a reusable HTTP client for fmsg-webapi.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// New creates a Client with the given base URL and JWT token.
func New(baseURL, token string) *Client {
	return &Client{
		BaseURL: baseURL,
		Token:   token,
		HTTP:    &http.Client{},
	}
}

// apiError represents an error response from the API.
type apiError struct {
	StatusCode int
	Body       string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("API error %d: %s", e.StatusCode, e.Body)
}

// do performs an HTTP request, attaching the Authorization header.
func (c *Client) do(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+c.Token)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("network error: %w", err)
	}
	return resp, nil
}

// checkStatus reads the response body and returns an error for non-2xx responses.
func checkStatus(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	msg := string(bytes.TrimSpace(body))
	if msg == "" {
		msg = http.StatusText(resp.StatusCode)
	}
	return &apiError{StatusCode: resp.StatusCode, Body: msg}
}

// Attachment represents a file attachment associated with a message.
type Attachment struct {
	Size     int    `json:"size"`
	Filename string `json:"filename"`
}

// MessageListItem represents a message in the list response.
type MessageListItem struct {
	ID          int64        `json:"id"`
	Version     int          `json:"version"`
	HasPID      bool         `json:"has_pid"`
	HasAddTo    bool         `json:"has_add_to"`
	Important   bool         `json:"important"`
	NoReply     bool         `json:"no_reply"`
	Deflate     bool         `json:"deflate"`
	PID         *int64       `json:"pid"`
	From        string       `json:"from"`
	To          []string     `json:"to"`
	AddTo       []string     `json:"add_to"`
	AddToFrom   *string      `json:"add_to_from"`
	Time        *float64     `json:"time"`
	Topic       string       `json:"topic"`
	Type        string       `json:"type"`
	Size        int          `json:"size"`
	Attachments []Attachment `json:"attachments"`
}

// Message represents a fmsg message as exchanged over the HTTP API.
type Message struct {
	Version     int          `json:"version"`
	HasPID      bool         `json:"has_pid"`
	HasAddTo    bool         `json:"has_add_to"`
	Important   bool         `json:"important"`
	NoReply     bool         `json:"no_reply"`
	Deflate     bool         `json:"deflate"`
	PID         *int64       `json:"pid"`
	From        string       `json:"from"`
	To          []string     `json:"to"`
	AddTo       []string     `json:"add_to"`
	AddToFrom   *string      `json:"add_to_from"`
	Time        *float64     `json:"time"`
	Topic       string       `json:"topic"`
	Type        string       `json:"type"`
	Size        int          `json:"size"`
	ShortText   *string      `json:"short_text"`
	Attachments []Attachment `json:"attachments"`
}

// CreateMessageResponse is the response from creating a message.
type CreateMessageResponse struct {
	ID int64 `json:"id"`
}

// SendMessageResponse is the response from sending a message.
type SendMessageResponse struct {
	ID   int64   `json:"id"`
	Time float64 `json:"time"`
}

// AddRecipientsResponse is the response from adding recipients to a message.
type AddRecipientsResponse struct {
	ID    int64 `json:"id"`
	Added int   `json:"added"`
}

// WaitResponse is the response from GET /fmsg/wait.
type WaitResponse struct {
	HasNew   bool  `json:"has_new"`
	LatestID int64 `json:"latest_id"`
}

// ListMessages returns messages for the authenticated user.
func (c *Client) ListMessages(limit, offset int) ([]MessageListItem, error) {
	u, err := url.Parse(c.BaseURL + "/fmsg")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		q.Set("offset", strconv.Itoa(offset))
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := checkStatus(resp); err != nil {
		return nil, err
	}

	var messages []MessageListItem
	if err := json.NewDecoder(resp.Body).Decode(&messages); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return messages, nil
}

// ListSentMessages returns messages authored by the authenticated user.
func (c *Client) ListSentMessages(limit, offset int) ([]MessageListItem, error) {
	u, err := url.Parse(c.BaseURL + "/fmsg/sent")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		q.Set("offset", strconv.Itoa(offset))
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := checkStatus(resp); err != nil {
		return nil, err
	}

	var messages []MessageListItem
	if err := json.NewDecoder(resp.Body).Decode(&messages); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return messages, nil
}

// WaitForMessage long-polls for a new message for the authenticated user.
func (c *Client) WaitForMessage(sinceID int64, timeout int) (*WaitResponse, error) {
	u, err := url.Parse(c.BaseURL + "/fmsg/wait")
	if err != nil {
		return nil, err
	}

	q := u.Query()
	q.Set("since_id", strconv.FormatInt(sinceID, 10))
	q.Set("timeout", strconv.Itoa(timeout))
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := checkStatus(resp); err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusNoContent {
		return &WaitResponse{HasNew: false}, nil
	}

	var result WaitResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &result, nil
}

// GetMessage retrieves a single message by ID.
func (c *Client) GetMessage(id string) (*Message, error) {
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+"/fmsg/"+id, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := checkStatus(resp); err != nil {
		return nil, err
	}

	var msg Message
	if err := json.NewDecoder(resp.Body).Decode(&msg); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &msg, nil
}

// CreateMessage creates a new draft message. body is optional JSON payload.
func (c *Client) CreateMessage(body []byte) (*CreateMessageResponse, error) {
	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/fmsg", bodyReader)
	if err != nil {
		return nil, err
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := checkStatus(resp); err != nil {
		return nil, err
	}

	var resp2 CreateMessageResponse
	if err := json.NewDecoder(resp.Body).Decode(&resp2); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &resp2, nil
}

// SendMessage sends a draft message by ID.
func (c *Client) SendMessage(id int64) (*SendMessageResponse, error) {
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/fmsg/"+strconv.FormatInt(id, 10)+"/send", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := checkStatus(resp); err != nil {
		return nil, err
	}

	var result SendMessageResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &result, nil
}

// AddRecipients adds additional recipients to an existing message.
func (c *Client) AddRecipients(id int64, addTo []string) (*AddRecipientsResponse, error) {
	body, err := json.Marshal(map[string]interface{}{"add_to": addTo})
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/fmsg/"+strconv.FormatInt(id, 10)+"/add-to", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := checkStatus(resp); err != nil {
		return nil, err
	}

	var result AddRecipientsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &result, nil
}

// DeleteMessage deletes a message by ID.
func (c *Client) DeleteMessage(id int64) error {
	req, err := http.NewRequest(http.MethodDelete, c.BaseURL+"/fmsg/"+strconv.FormatInt(id, 10), nil)
	if err != nil {
		return err
	}

	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return checkStatus(resp)
}

// UploadAttachment uploads a file as an attachment to a message using multipart.
func (c *Client) UploadAttachment(messageID, filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("opening file: %w", err)
	}
	defer f.Close()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return fmt.Errorf("creating form file: %w", err)
	}
	if _, err := io.Copy(fw, f); err != nil {
		return fmt.Errorf("reading file: %w", err)
	}
	if err := mw.Close(); err != nil {
		return fmt.Errorf("closing multipart writer: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/fmsg/"+messageID+"/attach", &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return checkStatus(resp)
}

// DownloadAttachment downloads an attachment and writes it to outputPath.
func (c *Client) DownloadAttachment(messageID, filename, outputPath string) error {
	req, err := http.NewRequest(http.MethodGet,
		c.BaseURL+"/fmsg/"+messageID+"/attach/"+filename, nil)
	if err != nil {
		return err
	}

	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := checkStatus(resp); err != nil {
		return err
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("writing output file: %w", err)
	}
	return nil
}

// DeleteAttachment removes an attachment from a message.
func (c *Client) DeleteAttachment(messageID, filename string) error {
	req, err := http.NewRequest(http.MethodDelete,
		c.BaseURL+"/fmsg/"+messageID+"/attach/"+filename, nil)
	if err != nil {
		return err
	}

	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return checkStatus(resp)
}

// UpdateMessage updates a draft message by ID.
func (c *Client) UpdateMessage(id int64, body []byte) error {
	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequest(http.MethodPut, c.BaseURL+"/fmsg/"+strconv.FormatInt(id, 10), bodyReader)
	if err != nil {
		return err
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return checkStatus(resp)
}

// DownloadDataToWriter downloads the message body data and writes it to w.
func (c *Client) DownloadDataToWriter(id string, w io.Writer) error {
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+"/fmsg/"+id+"/data", nil)
	if err != nil {
		return err
	}

	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := checkStatus(resp); err != nil {
		return err
	}

	if _, err := io.Copy(w, resp.Body); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}
	return nil
}

// DownloadData downloads the message body data and writes it to outputPath.
func (c *Client) DownloadData(id, outputPath string) error {
	out, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}
	defer out.Close()

	if err := c.DownloadDataToWriter(id, out); err != nil {
		return err
	}
	return nil
}
