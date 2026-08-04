// Package api provides an HTTP client for the fmsg-webapi service.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Client is a reusable HTTP client for fmsg-webapi.
type Client struct {
	BaseURL string
	Auth    TokenProvider
	HTTP    *http.Client
}

// TokenProvider returns an access token for authenticated API requests.
type TokenProvider interface {
	AccessToken(ctx context.Context, forceRefresh bool) (string, error)
}

// StaticTokenProvider is useful for tests that do not exercise auth refresh.
type StaticTokenProvider string

// AccessToken returns the static token.
func (p StaticTokenProvider) AccessToken(ctx context.Context, forceRefresh bool) (string, error) {
	return string(p), nil
}

// New creates a Client with the given base URL and token provider.
func New(baseURL string, auth TokenProvider) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Auth:    auth,
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

// do performs an HTTP request, attaching the Authorization header. If the API
// rejects the cached token, it refreshes once and retries replayable requests.
func (c *Client) do(req *http.Request) (*http.Response, error) {
	resp, err := c.doWithToken(req, false)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}

	if req.Body != nil && req.GetBody == nil {
		return resp, nil
	}
	resp.Body.Close()

	retry := req.Clone(req.Context())
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, fmt.Errorf("preparing request retry: %w", err)
		}
		retry.Body = body
		retry.GetBody = req.GetBody
	}

	resp, err = c.doWithToken(retry, true)
	if err != nil {
		return nil, fmt.Errorf("refreshing access token after 401: %w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		msg := string(bytes.TrimSpace(body))
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		return nil, fmt.Errorf("authentication failed after refreshing token; run fmsg login with valid credentials: %s", msg)
	}
	return resp, nil
}

func (c *Client) doWithToken(req *http.Request, forceRefresh bool) (*http.Response, error) {
	if c.Auth == nil {
		return nil, fmt.Errorf("missing token provider")
	}
	token, err := c.Auth.AccessToken(req.Context(), forceRefresh)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
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

// RecipientDelivery is the delivery state of a message for one recipient.
type RecipientDelivery struct {
	Addr          string  `json:"addr"`
	TimeDelivered *string `json:"time_delivered"`
	ResponseCode  *int    `json:"response_code"`
}

// AddToBatch is one batch of recipients added to a message in a single add-to call.
type AddToBatch struct {
	BatchID    int64               `json:"batch_id"`
	AddToFrom  string              `json:"add_to_from"`
	To         []string            `json:"to"`
	ToDelivery []RecipientDelivery `json:"to_delivery"`
	Time       float64             `json:"time"`
}

// MessageListItem represents a message in the list response.
type MessageListItem struct {
	ID          int64               `json:"id"`
	Version     int                 `json:"version"`
	HasPID      bool                `json:"has_pid"`
	HasAddTo    bool                `json:"has_add_to"`
	Important   bool                `json:"important"`
	NoReply     bool                `json:"no_reply"`
	Deflate     bool                `json:"deflate"`
	PID         *int64              `json:"pid"`
	From        string              `json:"from"`
	To          []string            `json:"to"`
	ToDelivery  []RecipientDelivery `json:"to_delivery"`
	AddTo       []AddToBatch        `json:"add_to"`
	Time        *float64            `json:"time"`
	Topic       string              `json:"topic"`
	Type        string              `json:"type"`
	Size        int                 `json:"size"`
	Read        bool                `json:"read"`
	TimeRead    *float64            `json:"time_read"`
	Attachments []Attachment        `json:"attachments"`
}

// Message represents a fmsg message as exchanged over the HTTP API.
type Message struct {
	Version     int                 `json:"version"`
	HasPID      bool                `json:"has_pid"`
	HasAddTo    bool                `json:"has_add_to"`
	Important   bool                `json:"important"`
	NoReply     bool                `json:"no_reply"`
	Deflate     bool                `json:"deflate"`
	PID         *int64              `json:"pid"`
	From        string              `json:"from"`
	To          []string            `json:"to"`
	ToDelivery  []RecipientDelivery `json:"to_delivery"`
	AddTo       []AddToBatch        `json:"add_to"`
	Time        *float64            `json:"time"`
	Topic       string              `json:"topic"`
	Type        string              `json:"type"`
	Size        int                 `json:"size"`
	ShortText   *string             `json:"short_text"`
	Read        bool                `json:"read"`
	TimeRead    *float64            `json:"time_read"`
	Attachments []Attachment        `json:"attachments"`
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

// SubAccount represents an API-access grant returned by the sub-accounts routes.
// APIKey is only populated once, immediately after creation or key rotation.
type SubAccount struct {
	Agent        string   `json:"agent"`
	Addr         string   `json:"addr"`
	GrantType    string   `json:"grant_type"`
	DisplayName  string   `json:"display_name,omitempty"`
	KeyID        string   `json:"key_id,omitempty"`
	AllowedCIDRs []string `json:"allowed_cidrs"`
	KeyExpiresAt string   `json:"key_expires_at"`
	APIKey       string   `json:"api_key,omitempty"`
}

// SubAccountList is the response from listing sub-accounts.
type SubAccountList struct {
	MaxSubAccounts int          `json:"max_sub_accounts"`
	SubAccounts    []SubAccount `json:"sub_accounts"`
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

// UploadAttachmentResponse is the response from uploading an attachment.
type UploadAttachmentResponse struct {
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
}

// UploadAttachment uploads a file as an attachment to a message using multipart.
func (c *Client) UploadAttachment(messageID, filePath string) (*UploadAttachmentResponse, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("opening file: %w", err)
	}
	defer f.Close()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return nil, fmt.Errorf("creating form file: %w", err)
	}
	if _, err := io.Copy(fw, f); err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}
	if err := mw.Close(); err != nil {
		return nil, fmt.Errorf("closing multipart writer: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/fmsg/"+messageID+"/attach", &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := checkStatus(resp); err != nil {
		return nil, err
	}

	var result UploadAttachmentResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &result, nil
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

// ListSubAccounts returns the API-access grants owned by the authenticated user.
func (c *Client) ListSubAccounts() (*SubAccountList, error) {
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+"/fmsg/sub-accounts", nil)
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

	var list SubAccountList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &list, nil
}

// GetSubAccount retrieves a single sub-account grant by agent name.
func (c *Client) GetSubAccount(agent string) (*SubAccount, error) {
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+"/fmsg/sub-accounts/"+url.PathEscape(agent), nil)
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

	var account SubAccount
	if err := json.NewDecoder(resp.Body).Decode(&account); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &account, nil
}

// CreateSubAccount derives a new sub-account and returns its plaintext API key.
func (c *Client) CreateSubAccount(agent string, allowedCIDRs []string, keyExpiresAt string) (*SubAccount, error) {
	payload, err := json.Marshal(map[string]interface{}{
		"agent":          agent,
		"allowed_cidrs":  allowedCIDRs,
		"key_expires_at": keyExpiresAt,
	})
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/fmsg/sub-accounts", bytes.NewReader(payload))
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

	var account SubAccount
	if err := json.NewDecoder(resp.Body).Decode(&account); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &account, nil
}

// UpdateSubAccountCIDRs replaces a sub-account's allowed CIDRs without rotating its key.
func (c *Client) UpdateSubAccountCIDRs(agent string, allowedCIDRs []string) (*SubAccount, error) {
	payload, err := json.Marshal(map[string]interface{}{
		"allowed_cidrs": allowedCIDRs,
	})
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPatch, c.BaseURL+"/fmsg/sub-accounts/"+url.PathEscape(agent), bytes.NewReader(payload))
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

	var account SubAccount
	if err := json.NewDecoder(resp.Body).Decode(&account); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &account, nil
}

// RotateSubAccountKey rotates a sub-account's API key and returns the new plaintext key.
func (c *Client) RotateSubAccountKey(agent, keyExpiresAt string) (*SubAccount, error) {
	payload, err := json.Marshal(map[string]interface{}{
		"key_expires_at": keyExpiresAt,
	})
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/fmsg/sub-accounts/"+url.PathEscape(agent)+"/rotate-key", bytes.NewReader(payload))
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

	var account SubAccount
	if err := json.NewDecoder(resp.Body).Decode(&account); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &account, nil
}

// DeleteSubAccount deletes a sub-account grant.
func (c *Client) DeleteSubAccount(agent string) error {
	req, err := http.NewRequest(http.MethodDelete, c.BaseURL+"/fmsg/sub-accounts/"+url.PathEscape(agent), nil)
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
