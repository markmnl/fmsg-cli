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
	"time"
)

const (
	// DefaultRequestTimeout is the default per-request timeout applied to
	// metadata and JSON API calls, and the ResponseHeaderTimeout for DefaultTransport.
	DefaultRequestTimeout = 30 * time.Second
	maxErrorBodyBytes     = 64 << 10
)

// DefaultTransport returns a clone of http.DefaultTransport configured with
// ResponseHeaderTimeout so stalled servers cannot hang indefinitely while
// streaming uploads and downloads remain unbounded by total transfer duration.
func DefaultTransport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.ResponseHeaderTimeout = DefaultRequestTimeout
	return t
}

// Client is a reusable HTTP client for fmsg-webapi.
type Client struct {
	BaseURL        string
	Auth           TokenProvider
	HTTP           *http.Client
	RequestTimeout time.Duration
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
		BaseURL:        strings.TrimRight(baseURL, "/"),
		Auth:           auth,
		HTTP:           &http.Client{Transport: DefaultTransport()},
		RequestTimeout: DefaultRequestTimeout,
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

func drainAndClose(rc io.ReadCloser) {
	if rc == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(rc, maxErrorBodyBytes))
	_ = rc.Close()
}

func readErrorBody(rc io.ReadCloser, statusCode int) string {
	var body []byte
	if rc != nil {
		body, _ = io.ReadAll(io.LimitReader(rc, maxErrorBodyBytes))
		drainAndClose(rc)
	}
	msg := string(bytes.TrimSpace(body))
	if msg == "" {
		msg = http.StatusText(statusCode)
	}
	return msg
}

func (c *Client) withRequestTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := c.RequestTimeout
	if timeout <= 0 {
		timeout = DefaultRequestTimeout
	}
	return context.WithTimeout(ctx, timeout)
}

func (c *Client) messagePath(id string) string {
	return c.BaseURL + "/fmsg/" + url.PathEscape(id)
}

func (c *Client) messageIDPath(id int64) string {
	return c.BaseURL + "/fmsg/" + strconv.FormatInt(id, 10)
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
	drainAndClose(resp.Body)

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
		msg := readErrorBody(resp.Body, resp.StatusCode)
		return nil, fmt.Errorf("authentication failed after refreshing token; run fmsg login with valid credentials: %s", msg)
	}
	return resp, nil
}

func (c *Client) doWithToken(req *http.Request, forceRefresh bool) (*http.Response, error) {
	if c.Auth == nil {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, fmt.Errorf("missing token provider")
	}
	token, err := c.Auth.AccessToken(req.Context(), forceRefresh)
	if err != nil {
		if req.Body != nil {
			_ = req.Body.Close()
		}
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

// checkStatus reads a bounded error body and returns an error for non-2xx responses.
func checkStatus(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return &apiError{StatusCode: resp.StatusCode, Body: readErrorBody(resp.Body, resp.StatusCode)}
}

// doJSON executes an API request with a request timeout and optional JSON body
// (either raw []byte or a value to marshal), checks status, and decodes the
// JSON response into out when non-nil.
func (c *Client) doJSON(ctx context.Context, method, rawURL string, body any, out any) (int, error) {
	ctx, cancel := c.withRequestTimeout(ctx)
	defer cancel()

	var payload []byte
	switch v := body.(type) {
	case nil:
	case []byte:
		payload = v
	default:
		var err error
		payload, err = json.Marshal(v)
		if err != nil {
			return 0, fmt.Errorf("encoding request: %w", err)
		}
	}

	var bodyReader io.Reader
	if len(payload) > 0 {
		bodyReader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, rawURL, bodyReader)
	if err != nil {
		return 0, err
	}
	if len(payload) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.do(req)
	if err != nil {
		return 0, err
	}
	defer drainAndClose(resp.Body)

	if err := checkStatus(resp); err != nil {
		return 0, err
	}

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return 0, fmt.Errorf("decoding response: %w", err)
		}
	}
	return resp.StatusCode, nil
}

func (c *Client) listMessages(ctx context.Context, path string, limit, offset int) ([]MessageListItem, error) {
	u, err := url.Parse(c.BaseURL + path)
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

	var messages []MessageListItem
	if _, err := c.doJSON(ctx, http.MethodGet, u.String(), nil, &messages); err != nil {
		return nil, err
	}
	return messages, nil
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
	Edit        bool                `json:"edit"`
	Edited      *Edited             `json:"edited"`
}

// Edited identifies the edit message in force on a message (FMSG-008). Size
// is the byte length of the edited data, nil when the edit could not be
// applied and the original data is shown instead.
type Edited struct {
	ID   int64   `json:"id"`
	Time float64 `json:"time"`
	Size *int    `json:"size"`
}

// VisibleMessages drops edit messages (FMSG-008) from items. They carry a
// change to another message, which the server already applies to that
// message, so they are not shown as messages in their own right.
func VisibleMessages(items []MessageListItem) []MessageListItem {
	visible := make([]MessageListItem, 0, len(items))
	for _, item := range items {
		if !item.Edit {
			visible = append(visible, item)
		}
	}
	return visible
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
	Edit        bool                `json:"edit"`
	Edited      *Edited             `json:"edited"`
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
func (c *Client) ListMessages(ctx context.Context, limit, offset int) ([]MessageListItem, error) {
	return c.listMessages(ctx, "/fmsg", limit, offset)
}

// ListSentMessages returns messages authored by the authenticated user.
func (c *Client) ListSentMessages(ctx context.Context, limit, offset int) ([]MessageListItem, error) {
	return c.listMessages(ctx, "/fmsg/sent", limit, offset)
}

// GetMessage retrieves a single message by ID.
func (c *Client) GetMessage(ctx context.Context, id string) (*Message, error) {
	var msg Message
	if _, err := c.doJSON(ctx, http.MethodGet, c.messagePath(id), nil, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// CreateMessage creates a new draft message. body is optional JSON payload.
func (c *Client) CreateMessage(ctx context.Context, body []byte) (*CreateMessageResponse, error) {
	var resp CreateMessageResponse
	if _, err := c.doJSON(ctx, http.MethodPost, c.BaseURL+"/fmsg", body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// SendMessage sends a draft message by ID.
func (c *Client) SendMessage(ctx context.Context, id int64) (*SendMessageResponse, error) {
	var result SendMessageResponse
	if _, err := c.doJSON(ctx, http.MethodPost, c.messageIDPath(id)+"/send", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// AddRecipients adds additional recipients to an existing message.
func (c *Client) AddRecipients(ctx context.Context, id int64, addTo []string) (*AddRecipientsResponse, error) {
	var result AddRecipientsResponse
	if _, err := c.doJSON(ctx, http.MethodPost, c.messageIDPath(id)+"/add-to", map[string]any{"add_to": addTo}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// EditMessageResponse identifies the edit message in force on a message after
// an edit call; ID, Time and SHA256 are nil when the message has none. Sent is
// false when the body already read that way and nothing was sent.
type EditMessageResponse struct {
	ID     *int64   `json:"id"`
	Time   *float64 `json:"time"`
	SHA256 *string  `json:"sha256"`
	Sent   bool     `json:"sent"`
}

// EditMessage replaces the body of a sent message the caller authored
// (FMSG-008), or reverts it to the original when data is nil.
func (c *Client) EditMessage(ctx context.Context, id int64, data *string) (*EditMessageResponse, error) {
	var result EditMessageResponse
	status, err := c.doJSON(ctx, http.MethodPost, c.messageIDPath(id)+"/edit", map[string]any{"data": data}, &result)
	if err != nil {
		return nil, err
	}
	result.Sent = status == http.StatusCreated
	return &result, nil
}

// DeleteMessage deletes a message by ID.
func (c *Client) DeleteMessage(ctx context.Context, id int64) error {
	_, err := c.doJSON(ctx, http.MethodDelete, c.messageIDPath(id), nil, nil)
	return err
}

// UploadAttachmentResponse is the response from uploading an attachment.
type UploadAttachmentResponse struct {
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
}

type multipartFileBody struct {
	io.Reader
	f *os.File
}

func (b *multipartFileBody) Close() error {
	return b.f.Close()
}

// UploadAttachment uploads a file as an attachment to a message using multipart.
// Regular files are streamed directly from disk with O(1) memory, an exact
// Content-Length, and a replayable GetBody so 401 token-refresh retries work.
// Non-regular files (such as named pipes) fall back to streaming without a
// fixed Content-Length.
func (c *Client) UploadAttachment(ctx context.Context, messageID, filePath string) (*UploadAttachmentResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("opening file: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("stat file: %w", err)
	}

	uploadURL := c.messagePath(messageID) + "/attach"
	baseName := filepath.Base(filePath)

	var req *http.Request
	if info.Mode().IsRegular() {
		var envelope bytes.Buffer
		mw := multipart.NewWriter(&envelope)
		if _, err := mw.CreateFormFile("file", baseName); err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("creating form file: %w", err)
		}
		headerLen := envelope.Len()
		if err := mw.Close(); err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("closing multipart writer: %w", err)
		}
		raw := envelope.Bytes()
		headerBytes := append([]byte(nil), raw[:headerLen]...)
		trailerBytes := append([]byte(nil), raw[headerLen:]...)
		fileSize := info.Size()

		body := &multipartFileBody{
			Reader: io.MultiReader(
				bytes.NewReader(headerBytes),
				io.LimitReader(f, fileSize),
				bytes.NewReader(trailerBytes),
			),
			f: f,
		}
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, body)
		if err != nil {
			_ = f.Close()
			return nil, err
		}
		req.ContentLength = int64(len(headerBytes)) + fileSize + int64(len(trailerBytes))
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.GetBody = func() (io.ReadCloser, error) {
			rf, err := os.Open(filePath)
			if err != nil {
				return nil, err
			}
			return &multipartFileBody{
				Reader: io.MultiReader(
					bytes.NewReader(headerBytes),
					io.LimitReader(rf, fileSize),
					bytes.NewReader(trailerBytes),
				),
				f: rf,
			}, nil
		}
	} else {
		pr, pw := io.Pipe()
		mw := multipart.NewWriter(pw)
		contentType := mw.FormDataContentType()
		go func() {
			defer f.Close()
			fw, err := mw.CreateFormFile("file", baseName)
			if err != nil {
				pw.CloseWithError(fmt.Errorf("creating form file: %w", err))
				return
			}
			if _, err := io.Copy(fw, f); err != nil {
				pw.CloseWithError(fmt.Errorf("reading file: %w", err))
				return
			}
			if err := mw.Close(); err != nil {
				pw.CloseWithError(fmt.Errorf("closing multipart writer: %w", err))
				return
			}
			_ = pw.Close()
		}()

		req, err = http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, pr)
		if err != nil {
			_ = pr.Close()
			return nil, err
		}
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer drainAndClose(resp.Body)

	if err := checkStatus(resp); err != nil {
		return nil, err
	}

	var result UploadAttachmentResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &result, nil
}

// downloadToFile fetches rawURL, verifies a 2xx status before touching the
// filesystem, streams into a temp file next to outputPath, and atomically
// renames it into place.
func (c *Client) downloadToFile(ctx context.Context, rawURL, outputPath string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}

	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer drainAndClose(resp.Body)

	if err := checkStatus(resp); err != nil {
		return err
	}

	out, err := os.CreateTemp(filepath.Dir(outputPath), ".fmsg-download-*")
	if err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}
	tmpPath := out.Name()
	success := false
	defer func() {
		if !success {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := io.Copy(out, resp.Body); err != nil {
		_ = out.Close()
		return fmt.Errorf("writing output file: %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("closing output file: %w", err)
	}
	if err := os.Rename(tmpPath, outputPath); err != nil {
		return fmt.Errorf("saving output file: %w", err)
	}
	success = true
	return nil
}

// DownloadAttachment downloads an attachment and writes it atomically to outputPath.
func (c *Client) DownloadAttachment(ctx context.Context, messageID, filename, outputPath string) error {
	rawURL := c.messagePath(messageID) + "/attach/" + url.PathEscape(filename)
	return c.downloadToFile(ctx, rawURL, outputPath)
}

// DeleteAttachment removes an attachment from a message.
func (c *Client) DeleteAttachment(ctx context.Context, messageID, filename string) error {
	rawURL := c.messagePath(messageID) + "/attach/" + url.PathEscape(filename)
	_, err := c.doJSON(ctx, http.MethodDelete, rawURL, nil, nil)
	return err
}

// UpdateMessage updates a draft message by ID.
func (c *Client) UpdateMessage(ctx context.Context, id int64, body []byte) error {
	_, err := c.doJSON(ctx, http.MethodPut, c.messageIDPath(id), body, nil)
	return err
}

// DownloadDataToWriter downloads the message body data and writes it to w.
// An edited message (FMSG-008) downloads as edited unless original is set.
func (c *Client) DownloadDataToWriter(ctx context.Context, id string, original bool, w io.Writer) error {
	if ctx == nil {
		ctx = context.Background()
	}
	rawURL := c.messagePath(id) + "/data"
	if original {
		rawURL += "?original=true"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}

	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer drainAndClose(resp.Body)

	if err := checkStatus(resp); err != nil {
		return err
	}

	if _, err := io.Copy(w, resp.Body); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}
	return nil
}

// DownloadData downloads the message body data and writes it atomically to outputPath.
func (c *Client) DownloadData(ctx context.Context, id string, original bool, outputPath string) error {
	rawURL := c.messagePath(id) + "/data"
	if original {
		rawURL += "?original=true"
	}
	return c.downloadToFile(ctx, rawURL, outputPath)
}

// ListSubAccounts returns the API-access grants owned by the authenticated user.
func (c *Client) ListSubAccounts(ctx context.Context) (*SubAccountList, error) {
	var list SubAccountList
	if _, err := c.doJSON(ctx, http.MethodGet, c.BaseURL+"/fmsg/sub-accounts", nil, &list); err != nil {
		return nil, err
	}
	return &list, nil
}

// GetSubAccount retrieves a single sub-account grant by agent name.
func (c *Client) GetSubAccount(ctx context.Context, agent string) (*SubAccount, error) {
	var account SubAccount
	if _, err := c.doJSON(ctx, http.MethodGet, c.BaseURL+"/fmsg/sub-accounts/"+url.PathEscape(agent), nil, &account); err != nil {
		return nil, err
	}
	return &account, nil
}

// CreateSubAccount derives a new sub-account and returns its plaintext API key.
func (c *Client) CreateSubAccount(ctx context.Context, agent string, allowedCIDRs []string, keyExpiresAt string) (*SubAccount, error) {
	var account SubAccount
	payload := map[string]any{
		"agent":          agent,
		"allowed_cidrs":  allowedCIDRs,
		"key_expires_at": keyExpiresAt,
	}
	if _, err := c.doJSON(ctx, http.MethodPost, c.BaseURL+"/fmsg/sub-accounts", payload, &account); err != nil {
		return nil, err
	}
	return &account, nil
}

// UpdateSubAccountCIDRs replaces a sub-account's allowed CIDRs without rotating its key.
func (c *Client) UpdateSubAccountCIDRs(ctx context.Context, agent string, allowedCIDRs []string) (*SubAccount, error) {
	var account SubAccount
	payload := map[string]any{
		"allowed_cidrs": allowedCIDRs,
	}
	if _, err := c.doJSON(ctx, http.MethodPatch, c.BaseURL+"/fmsg/sub-accounts/"+url.PathEscape(agent), payload, &account); err != nil {
		return nil, err
	}
	return &account, nil
}

// RotateSubAccountKey rotates a sub-account's API key and returns the new plaintext key.
func (c *Client) RotateSubAccountKey(ctx context.Context, agent, keyExpiresAt string) (*SubAccount, error) {
	var account SubAccount
	payload := map[string]any{
		"key_expires_at": keyExpiresAt,
	}
	if _, err := c.doJSON(ctx, http.MethodPost, c.BaseURL+"/fmsg/sub-accounts/"+url.PathEscape(agent)+"/rotate-key", payload, &account); err != nil {
		return nil, err
	}
	return &account, nil
}

// DeleteSubAccount deletes a sub-account grant.
func (c *Client) DeleteSubAccount(ctx context.Context, agent string) error {
	_, err := c.doJSON(ctx, http.MethodDelete, c.BaseURL+"/fmsg/sub-accounts/"+url.PathEscape(agent), nil, nil)
	return err
}
