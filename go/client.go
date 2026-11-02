// Package bbmask is the Go client for the Data Mask anonymization service.
// It never calls a model. Init runs once; Anonymize and Restore go to the service.
//
// It uses the same endpoints as the Python and Node packages. Errors are
// returned as *BBError. There are no separate async functions; calls block.
//
// AnonymizeFile(path) reads a path; AnonymizeBytes(filename, content) takes bytes.
// Both names mean the same thing on the package and on BBAnonym.
package bbmask

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	Version        = "0.3.1"
	DefaultBaseURL = "https://app.bbmask.co"
	// The engine waits 120 s; the client gives up after it so the refund/502 decision comes from the server.
	DefaultTimeout = 150 * time.Second
	StyleAngle     = "angle"
	StyleBracketTR = "bracket_tr"
)

// BBError is a service or client error. Error() is the detail text.
type BBError struct {
	StatusCode int
	Detail     string
	RequestID  string
}

func (e *BBError) Error() string {
	if e == nil {
		return ""
	}
	return e.Detail
}

func bbError(status int, detail string) *BBError {
	return &BBError{StatusCode: status, Detail: detail}
}

// Tagged is the result of an anonymize call.
type Tagged struct {
	Text             string
	DocID            string
	State            map[string]any
	EntitiesFound    map[string]int
	BalanceCredits   string
	RequestID        string
	PlaceholderStyle string
	KeepLegalForm    bool
	HealthData       bool
	LLMReview        map[string]any
}

// Config holds the address and timeout for Init. Empty fields keep the defaults.
type Config struct {
	BaseURL string
	Timeout time.Duration
}

// Option is a named option for Anonymize, the file calls and Session.
type Option func(*callOptions)

type callOptions struct {
	persist          bool
	placeholderStyle string
	keepLegalForm    bool
	healthData       bool
	llmReview        bool
}

// WithPersist chooses whether the mapping is kept on the service. Default true.
func WithPersist(persist bool) Option {
	return func(o *callOptions) { o.persist = persist }
}

// WithPlaceholderStyle accepts "angle" or "bracket_tr".
func WithPlaceholderStyle(style string) Option {
	return func(o *callOptions) { o.placeholderStyle = style }
}

// WithKeepLegalForm keeps the company type suffix outside the tag.
func WithKeepLegalForm(keep bool) Option {
	return func(o *callOptions) { o.keepLegalForm = keep }
}

// WithHealthData also tags diagnoses, drugs, lab results and blood type.
func WithHealthData(health bool) Option {
	return func(o *callOptions) { o.healthData = health }
}

// WithLLMReview requests a second pass for entities the rules missed.
func WithLLMReview(review bool) Option {
	return func(o *callOptions) { o.llmReview = review }
}

func resolve(opts ...Option) (callOptions, error) {
	o := callOptions{persist: true, placeholderStyle: StyleAngle}
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	if o.placeholderStyle != StyleAngle && o.placeholderStyle != StyleBracketTR {
		return callOptions{}, bbError(400, "placeholder_style must be angle or bracket_tr")
	}
	return o, nil
}

// BBAnonym is the client for a single key. No Init needed.
// The key is not an exported field; it stays out of %+v and log output.
type BBAnonym struct {
	BaseURL    string
	Timeout    time.Duration
	token      string
	httpClient *http.Client
}

// String masks the key; the full key never shows in fmt or log output.
func (c BBAnonym) String() string {
	masked := "***"
	if len(c.token) > 12 {
		masked = c.token[:8] + "…" + c.token[len(c.token)-4:]
	}
	return fmt.Sprintf("BBAnonym{BaseURL:%s Token:%s Timeout:%s}", c.BaseURL, masked, c.Timeout)
}

// New creates a client. If timeout is missing or zero, DefaultTimeout is used.
func New(baseURL, token string, timeout ...time.Duration) *BBAnonym {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = DefaultBaseURL
	}
	limit := DefaultTimeout
	if len(timeout) > 0 && timeout[0] > 0 {
		limit = timeout[0]
	}
	return &BBAnonym{
		BaseURL: base,
		token:   token,
		Timeout: limit,
		httpClient: &http.Client{
			Timeout: limit,
		},
	}
}

// Anonymize anonymizes text. persist defaults to true: DocID is set.
// WithPersist(false) stores no mapping: State is set and DocID stays empty.
func (c *BBAnonym) Anonymize(text string, opts ...Option) (*Tagged, error) {
	o, err := resolve(opts...)
	if err != nil {
		return nil, err
	}
	return c.anonymize(text, o)
}

func (c *BBAnonym) anonymize(text string, o callOptions) (*Tagged, error) {
	body, err := c.postJSON("/v1/anonymize", map[string]any{
		"text":              text,
		"persist":           o.persist,
		"placeholder_style": o.placeholderStyle,
		"keep_legal_form":   o.keepLegalForm,
		"health_data":       o.healthData,
		"llm_review":        o.llmReview,
	})
	if err != nil {
		return nil, err
	}
	return toTagged(body, o.persist), nil
}

// AnonymizeBytes sends one file from bytes (.txt, .pdf, .docx).
func (c *BBAnonym) AnonymizeBytes(filename string, content []byte, opts ...Option) (*Tagged, error) {
	o, err := resolve(opts...)
	if err != nil {
		return nil, err
	}
	return c.anonymizeBytes(filename, content, o)
}

func (c *BBAnonym) anonymizeBytes(filename string, content []byte, o callOptions) (*Tagged, error) {
	body, err := c.postForm("/v1/anonymize/file", o, []formFile{{
		field:   "file",
		name:    filename,
		content: content,
	}})
	if err != nil {
		return nil, err
	}
	return toTagged(body, o.persist), nil
}

// AnonymizeFile reads a path and sends one file.
func (c *BBAnonym) AnonymizeFile(path string, opts ...Option) (*Tagged, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return c.AnonymizeBytes(filepath.Base(path), content, opts...)
}

// AnonymizeDocuments anonymizes 1–3 files under one DocID.
func (c *BBAnonym) AnonymizeDocuments(paths []string, opts ...Option) (*Tagged, error) {
	o, err := resolve(opts...)
	if err != nil {
		return nil, err
	}
	return c.anonymizeDocuments(paths, o)
}

func (c *BBAnonym) anonymizeDocuments(paths []string, o callOptions) (*Tagged, error) {
	if len(paths) < 1 || len(paths) > 3 {
		return nil, bbError(400, "1–3 files per request")
	}
	files := make([]formFile, 0, len(paths))
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		files = append(files, formFile{field: "files", name: filepath.Base(path), content: content})
	}
	body, err := c.postForm("/v1/anonymize/documents", o, files)
	if err != nil {
		return nil, err
	}
	return toTagged(body, o.persist), nil
}

// Restore puts the real values back into the model text using a doc_id (string) or a state (map).
func (c *BBAnonym) Restore(docIDOrState any, modelText string) (string, error) {
	payload := map[string]any{"text": modelText}
	if state, ok := stateOf(docIDOrState); ok {
		payload["state"] = state
	} else if id, ok := docIDOrState.(string); ok {
		payload["doc_id"] = id
	} else {
		return "", bbError(400, "restore needs a doc_id (string) or a state (map)")
	}
	body, err := c.postJSON("/v1/restore", payload)
	if err != nil {
		return "", err
	}
	return stringField(body, "text"), nil
}

// GetMap returns the dictionary from the service. With once true the service copy is deleted after reading.
func (c *BBAnonym) GetMap(docID string, once bool) (map[string]any, error) {
	query := url.Values{}
	query.Set("once", strconv.FormatBool(once))
	body, err := c.do(http.MethodGet, "/v1/mappings/"+url.PathEscape(docID), query, nil, "")
	if err != nil {
		return nil, err
	}
	state, _ := body["state"].(map[string]any)
	if state == nil {
		return map[string]any{}, nil
	}
	return state, nil
}

// Delete removes the mapping from the service. Returns true if the service reports deleted.
func (c *BBAnonym) Delete(docID string) (bool, error) {
	body, err := c.do(http.MethodDelete, "/v1/mappings/"+url.PathEscape(docID), nil, nil, "")
	if err != nil {
		return false, err
	}
	deleted, _ := body["deleted"].(bool)
	return deleted, nil
}

type formFile struct {
	field   string
	name    string
	content []byte
}

func (c *BBAnonym) postJSON(path string, payload any) (map[string]any, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return c.do(http.MethodPost, path, nil, bytes.NewReader(raw), "application/json")
}

func (c *BBAnonym) postForm(path string, o callOptions, files []formFile) (map[string]any, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	fields := [][2]string{
		{"persist", strconv.FormatBool(o.persist)},
		{"placeholder_style", o.placeholderStyle},
		{"keep_legal_form", strconv.FormatBool(o.keepLegalForm)},
		{"health_data", strconv.FormatBool(o.healthData)},
		{"llm_review", strconv.FormatBool(o.llmReview)},
	}
	for _, field := range fields {
		if err := writer.WriteField(field[0], field[1]); err != nil {
			return nil, err
		}
	}
	for _, file := range files {
		part, err := writer.CreateFormFile(file.field, file.name)
		if err != nil {
			return nil, err
		}
		if _, err := part.Write(file.content); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return c.do(http.MethodPost, path, nil, &buf, writer.FormDataContentType())
}

func (c *BBAnonym) do(method, path string, query url.Values, body io.Reader, contentType string) (map[string]any, error) {
	endpoint := c.BaseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		return nil, bbError(503, fmt.Sprintf("Could not reach the service (%s)", c.BaseURL))
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("User-Agent", "bb-mask-go/"+Version)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, c.transportError(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, c.transportError(err)
	}
	return parseBody(resp.StatusCode, resp.Header.Get("X-Request-Id"), raw)
}

func (c *BBAnonym) transportError(err error) *BBError {
	if isTimeout(err) {
		return bbError(504, fmt.Sprintf("Timed out (%s s). The fee may have been charged.", formatSeconds(c.Timeout)))
	}
	return bbError(503, fmt.Sprintf("Could not reach the service (%s)", c.BaseURL))
}

func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func formatSeconds(d time.Duration) string {
	text := strconv.FormatFloat(d.Seconds(), 'f', 3, 64)
	text = strings.TrimRight(text, "0")
	text = strings.TrimRight(text, ".")
	if text == "" || text == "-" {
		return "0"
	}
	return text
}

func parseBody(status int, requestID string, raw []byte) (map[string]any, error) {
	if status >= 400 {
		return nil, &BBError{StatusCode: status, Detail: errorDetail(raw), RequestID: requestID}
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil || body == nil {
		return nil, &BBError{StatusCode: 502, Detail: "The service did not return JSON", RequestID: requestID}
	}
	body["_request_id"] = requestID
	return body, nil
}

func errorDetail(raw []byte) string {
	detail := string(raw)
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil || body == nil {
		return detail
	}
	value, ok := body["detail"]
	if !ok || value == nil {
		return detail
	}
	if text, ok := value.(string); ok {
		return text
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return detail
	}
	return string(encoded)
}

func toTagged(body map[string]any, persist bool) *Tagged {
	docID := stringField(body, "doc_id")
	if !persist {
		docID = ""
	}
	style := stringField(body, "placeholder_style")
	if style == "" {
		style = StyleAngle
	}
	state, _ := body["state"].(map[string]any)
	var review map[string]any
	if raw, ok := body["llm_review"].(map[string]any); ok {
		review = raw
	}
	return &Tagged{
		Text:             stringField(body, "text"),
		DocID:            docID,
		State:            state,
		EntitiesFound:    intMap(body["entities_found"]),
		BalanceCredits:   stringField(body, "balance_credits"),
		RequestID:        stringField(body, "_request_id"),
		PlaceholderStyle: style,
		KeepLegalForm:    boolField(body["keep_legal_form"]),
		HealthData:       boolField(body["health_data"]),
		LLMReview:        review,
	}
}

func stringField(body map[string]any, key string) string {
	text, _ := body[key].(string)
	return text
}

func boolField(value any) bool {
	flag, _ := value.(bool)
	return flag
}

func intMap(value any) map[string]int {
	out := map[string]int{}
	raw, ok := value.(map[string]any)
	if !ok {
		return out
	}
	for key, item := range raw {
		switch n := item.(type) {
		case float64:
			out[key] = int(n)
		case int:
			out[key] = n
		case json.Number:
			parsed, err := n.Int64()
			if err == nil {
				out[key] = int(parsed)
			}
		}
	}
	return out
}

func stateOf(value any) (any, bool) {
	if value == nil {
		return nil, false
	}
	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Map || rv.IsNil() {
		return nil, false
	}
	return value, true
}

// MaskSession keeps the DocID or state inside. Close deletes the mapping on exit.
type MaskSession struct {
	Persist bool

	client *BBAnonym
	opts   callOptions
	ids    []string
	docID  string
	state  map[string]any
}

// Session opens a session on the process client. Options apply to every anonymize call.
func Session(opts ...Option) (*MaskSession, error) {
	client, err := requireClient()
	if err != nil {
		return nil, err
	}
	resolved, err := resolve(opts...)
	if err != nil {
		return nil, err
	}
	return &MaskSession{Persist: resolved.persist, client: client, opts: resolved}, nil
}

// Anonymize anonymizes text with the session's persist and options.
func (s *MaskSession) Anonymize(text string) (*Tagged, error) {
	tagged, err := s.client.anonymize(text, s.opts)
	if err != nil {
		return nil, err
	}
	return s.remember(tagged), nil
}

// AnonymizeFile reads a path.
func (s *MaskSession) AnonymizeFile(path string) (*Tagged, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	tagged, err := s.client.anonymizeBytes(filepath.Base(path), content, s.opts)
	if err != nil {
		return nil, err
	}
	return s.remember(tagged), nil
}

// AnonymizeDocuments sends 1–3 files with the session options.
func (s *MaskSession) AnonymizeDocuments(paths []string) (*Tagged, error) {
	tagged, err := s.client.anonymizeDocuments(paths, s.opts)
	if err != nil {
		return nil, err
	}
	return s.remember(tagged), nil
}

// Restore takes only the model text. Returns 400 if nothing was anonymized.
func (s *MaskSession) Restore(modelText string) (string, error) {
	if s.Persist {
		if s.docID == "" {
			return "", bbError(400, "Nothing anonymized in this session")
		}
		return s.client.Restore(s.docID, modelText)
	}
	if len(s.state) == 0 {
		return "", bbError(400, "No state in this session")
	}
	return s.client.Restore(s.state, modelText)
}

// Close calls Delete for every kept DocID when persist is true.
// When persist is false the state is dropped and no delete is sent.
// A BBError is swallowed; network errors are BBError too, so Close usually returns nil.
func (s *MaskSession) Close() error {
	if !s.Persist {
		s.state = nil
		return nil
	}
	for _, docID := range s.ids {
		if _, err := s.client.Delete(docID); err != nil {
			var bb *BBError
			if !errors.As(err, &bb) {
				return err
			}
		}
	}
	s.ids = nil
	return nil
}

func (s *MaskSession) remember(tagged *Tagged) *Tagged {
	s.docID = tagged.DocID
	s.state = tagged.State
	if tagged.DocID != "" {
		s.ids = append(s.ids, tagged.DocID)
	}
	return tagged
}

var (
	mu      sync.RWMutex
	current *BBAnonym
)

// Init sets up the process client. A second call replaces the first.
// An empty key returns 401 and leaves the existing client unchanged.
func Init(apiKey string, cfg ...Config) (*BBAnonym, error) {
	key := strings.TrimSpace(apiKey)
	if key == "" {
		return nil, bbError(401, "api_key is empty")
	}
	baseURL := DefaultBaseURL
	timeout := DefaultTimeout
	if len(cfg) > 0 {
		if strings.TrimSpace(cfg[0].BaseURL) != "" {
			baseURL = cfg[0].BaseURL
		}
		if cfg[0].Timeout > 0 {
			timeout = cfg[0].Timeout
		}
	}
	client := New(baseURL, key, timeout)
	mu.Lock()
	current = client
	mu.Unlock()
	return client, nil
}

func requireClient() (*BBAnonym, error) {
	mu.RLock()
	defer mu.RUnlock()
	if current == nil {
		return nil, bbError(401, "Init was not called")
	}
	return current, nil
}

// Anonymize anonymizes text with the process client. Requires Init.
func Anonymize(text string, opts ...Option) (*Tagged, error) {
	client, err := requireClient()
	if err != nil {
		return nil, err
	}
	return client.Anonymize(text, opts...)
}

// AnonymizeFile reads a path.
func AnonymizeFile(path string, opts ...Option) (*Tagged, error) {
	client, err := requireClient()
	if err != nil {
		return nil, err
	}
	return client.AnonymizeFile(path, opts...)
}

// AnonymizeBytes sends one file from bytes.
func AnonymizeBytes(filename string, content []byte, opts ...Option) (*Tagged, error) {
	client, err := requireClient()
	if err != nil {
		return nil, err
	}
	return client.AnonymizeBytes(filename, content, opts...)
}

// AnonymizeDocuments sends 1–3 files with the process client.
func AnonymizeDocuments(paths []string, opts ...Option) (*Tagged, error) {
	client, err := requireClient()
	if err != nil {
		return nil, err
	}
	return client.AnonymizeDocuments(paths, opts...)
}

// Restore puts the real values back with the process client.
func Restore(docIDOrState any, modelText string) (string, error) {
	client, err := requireClient()
	if err != nil {
		return "", err
	}
	return client.Restore(docIDOrState, modelText)
}

// GetMap reads the dictionary with the process client.
func GetMap(docID string, once bool) (map[string]any, error) {
	client, err := requireClient()
	if err != nil {
		return nil, err
	}
	return client.GetMap(docID, once)
}

// Delete removes the mapping with the process client.
func Delete(docID string) (bool, error) {
	client, err := requireClient()
	if err != nil {
		return false, err
	}
	return client.Delete(docID)
}
