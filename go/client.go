// Package bbmask Data Mask anonimleştirme servisinin Go istemcisidir.
// Model çağırmaz. Init bir kez çalışır; Anonymize ve Restore servise gider.
//
// Python ve Node paketleriyle aynı uçları kullanır. Hatalar *BBError olarak
// error döner. Ayrı async fonksiyon yoktur; çağrı bloklar.
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
	Version        = "0.2.0"
	DefaultBaseURL = "https://app.bbmask.co"
	DefaultTimeout = 120 * time.Second
	StyleAngle     = "angle"
	StyleBracketTR = "bracket_tr"

	notOpenDetail = "Bu uç henüz açık değil."
)

// BBError servis veya istemci hatasıdır. Error() ayrıntı metnidir.
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

// Tagged anonymize çağrısının dönüşüdür.
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

// Config Init için adres ve zaman aşımıdır. Boş alanlar varsayılanı korur.
type Config struct {
	BaseURL string
	Timeout time.Duration
}

// Option Anonymize, dosya çağrıları ve Session için isimli seçenektir.
type Option func(*callOptions)

type callOptions struct {
	persist          bool
	placeholderStyle string
	keepLegalForm    bool
	healthData       bool
	llmReview        bool
}

// WithPersist haritanın serviste tutulup tutulmayacağını seçer. Varsayılan true.
func WithPersist(persist bool) Option {
	return func(o *callOptions) { o.persist = persist }
}

// WithPlaceholderStyle "angle" veya "bracket_tr" kabul eder.
func WithPlaceholderStyle(style string) Option {
	return func(o *callOptions) { o.placeholderStyle = style }
}

// WithKeepLegalForm şirket türü ekini etiketin dışında bırakır.
func WithKeepLegalForm(keep bool) Option {
	return func(o *callOptions) { o.keepLegalForm = keep }
}

// WithHealthData tanı, ilaç, tahlil ve kan grubunu da etiketler.
func WithHealthData(health bool) Option {
	return func(o *callOptions) { o.healthData = health }
}

// WithLLMReview kuralların kaçırdığı varlıklar için ikinci bir geçiş ister.
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
		return callOptions{}, bbError(400, "placeholder_style angle veya bracket_tr olmalı")
	}
	return o, nil
}

// BBAnonym tek bir anahtarın istemcisidir. Init gerekmez.
type BBAnonym struct {
	BaseURL    string
	Token      string
	Timeout    time.Duration
	httpClient *http.Client
}

// New istemci kurar. timeout verilmezse veya sıfırsa DefaultTimeout kullanılır.
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
		Token:   token,
		Timeout: limit,
		httpClient: &http.Client{
			Timeout: limit,
		},
	}
}

// Anonymize metni anonimleştirir. Varsayılan persist true: DocID dolar.
// WithPersist(false) haritayı yazmaz, State dolar, DocID boş kalır.
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

// AnonymizeFile bayttan tek dosya gönderir (.txt, .pdf, .docx).
func (c *BBAnonym) AnonymizeFile(filename string, content []byte, opts ...Option) (*Tagged, error) {
	o, err := resolve(opts...)
	if err != nil {
		return nil, err
	}
	return c.anonymizeFile(filename, content, o)
}

func (c *BBAnonym) anonymizeFile(filename string, content []byte, o callOptions) (*Tagged, error) {
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

// AnonymizePath yolu okur ve tek dosya gönderir.
func (c *BBAnonym) AnonymizePath(path string, opts ...Option) (*Tagged, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return c.AnonymizeFile(filepath.Base(path), content, opts...)
}

// AnonymizeDocuments 1–3 dosyayı tek DocID altında anonimleştirir.
func (c *BBAnonym) AnonymizeDocuments(paths []string, opts ...Option) (*Tagged, error) {
	o, err := resolve(opts...)
	if err != nil {
		return nil, err
	}
	return c.anonymizeDocuments(paths, o)
}

func (c *BBAnonym) anonymizeDocuments(paths []string, o callOptions) (*Tagged, error) {
	if len(paths) < 1 || len(paths) > 3 {
		return nil, bbError(400, "Bir istekte 1–3 dosya")
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

// Restore doc_id (string) veya state (map) ile model metnini açar.
func (c *BBAnonym) Restore(docIDOrState any, modelText string) (string, error) {
	payload := map[string]any{"text": modelText}
	if state, ok := stateOf(docIDOrState); ok {
		payload["state"] = state
	} else if id, ok := docIDOrState.(string); ok {
		payload["doc_id"] = id
	} else {
		return "", bbError(400, "restore doc_id (string) veya state (map) ister")
	}
	body, err := c.postJSON("/v1/restore", payload)
	if err != nil {
		return "", err
	}
	return stringField(body, "text"), nil
}

// GetMap servisteki sözlüğü döner. once true ise okuyunca kopyayı siler.
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

// Delete servisteki haritayı siler. Servis deleted derse true döner.
func (c *BBAnonym) Delete(docID string) (bool, error) {
	body, err := c.do(http.MethodDelete, "/v1/mappings/"+url.PathEscape(docID), nil, nil, "")
	if err != nil {
		return false, err
	}
	deleted, _ := body["deleted"].(bool)
	return deleted, nil
}

// Complete henüz açık değil. HTTP çağrısı yapmaz, 501 döner.
func (c *BBAnonym) Complete(text string) (string, error) {
	return "", bbError(501, notOpenDetail)
}

// CompleteFile henüz açık değil. HTTP çağrısı yapmaz, 501 döner.
func (c *BBAnonym) CompleteFile(path string) (string, error) {
	return "", bbError(501, notOpenDetail)
}

// CompleteDocuments henüz açık değil. HTTP çağrısı yapmaz, 501 döner.
func (c *BBAnonym) CompleteDocuments(paths []string) (string, error) {
	return "", bbError(501, notOpenDetail)
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
		return nil, bbError(503, fmt.Sprintf("Servise ulaşılamadı (%s)", c.BaseURL))
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
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
		return bbError(504, fmt.Sprintf("Zaman aşımı (%s sn). Ücret düşmüş olabilir.", formatSeconds(c.Timeout)))
	}
	return bbError(503, fmt.Sprintf("Servise ulaşılamadı (%s)", c.BaseURL))
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
		return nil, &BBError{StatusCode: 502, Detail: "Servis JSON dönmedi", RequestID: requestID}
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

// MaskSession DocID veya state'i içinde tutar. Close çıkışta haritayı siler.
type MaskSession struct {
	Persist bool

	client *BBAnonym
	opts   callOptions
	ids    []string
	docID  string
	state  map[string]any
}

// Session süreç istemcisinden oturum açar. Seçenekler her anonymize çağrısına gider.
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

// Anonymize oturumun persist ve seçenekleriyle metni anonimleştirir.
func (s *MaskSession) Anonymize(text string) (*Tagged, error) {
	tagged, err := s.client.anonymize(text, s.opts)
	if err != nil {
		return nil, err
	}
	return s.remember(tagged), nil
}

// AnonymizeFile yolu okur.
func (s *MaskSession) AnonymizeFile(path string) (*Tagged, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	tagged, err := s.client.anonymizeFile(filepath.Base(path), content, s.opts)
	if err != nil {
		return nil, err
	}
	return s.remember(tagged), nil
}

// AnonymizeDocuments 1–3 dosyayı oturum seçenekleriyle gönderir.
func (s *MaskSession) AnonymizeDocuments(paths []string) (*Tagged, error) {
	tagged, err := s.client.anonymizeDocuments(paths, s.opts)
	if err != nil {
		return nil, err
	}
	return s.remember(tagged), nil
}

// Restore yalnız model metnini alır. Anonymize yoksa 400 döner.
func (s *MaskSession) Restore(modelText string) (string, error) {
	if s.Persist {
		if s.docID == "" {
			return "", bbError(400, "Oturumda anonymize yok")
		}
		return s.client.Restore(s.docID, modelText)
	}
	if len(s.state) == 0 {
		return "", bbError(400, "Oturumda state yok")
	}
	return s.client.Restore(s.state, modelText)
}

// Close persist true ise tutulan her DocID için Delete çağırır.
// persist false ise state düşer, servise silme gitmez.
// BBError yutulur; ağ hataları da BBError olduğundan çıkış genelde nil döner.
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

// Init süreç istemcisini kurar. İkinci çağrı öncekinin yerine geçer.
// Boş anahtar 401 döner ve kurulu istemciyi değiştirmez.
func Init(apiKey string, cfg ...Config) (*BBAnonym, error) {
	key := strings.TrimSpace(apiKey)
	if key == "" {
		return nil, bbError(401, "api_key boş")
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
		return nil, bbError(401, "Init çağrılmadı")
	}
	return current, nil
}

// Anonymize süreç istemcisinde metni anonimleştirir. Init gerekir.
func Anonymize(text string, opts ...Option) (*Tagged, error) {
	client, err := requireClient()
	if err != nil {
		return nil, err
	}
	return client.Anonymize(text, opts...)
}

// AnonymizeFile yolu okur. Bayt sizdeyse BBAnonym.AnonymizeFile kullanın.
func AnonymizeFile(path string, opts ...Option) (*Tagged, error) {
	client, err := requireClient()
	if err != nil {
		return nil, err
	}
	return client.AnonymizePath(path, opts...)
}

// AnonymizeDocuments 1–3 dosyayı süreç istemcisiyle gönderir.
func AnonymizeDocuments(paths []string, opts ...Option) (*Tagged, error) {
	client, err := requireClient()
	if err != nil {
		return nil, err
	}
	return client.AnonymizeDocuments(paths, opts...)
}

// Restore süreç istemcisinde etiketi açar.
func Restore(docIDOrState any, modelText string) (string, error) {
	client, err := requireClient()
	if err != nil {
		return "", err
	}
	return client.Restore(docIDOrState, modelText)
}

// GetMap süreç istemcisinde sözlüğü okur.
func GetMap(docID string, once bool) (map[string]any, error) {
	client, err := requireClient()
	if err != nil {
		return nil, err
	}
	return client.GetMap(docID, once)
}

// Delete süreç istemcisinde haritayı siler.
func Delete(docID string) (bool, error) {
	client, err := requireClient()
	if err != nil {
		return false, err
	}
	return client.Delete(docID)
}

// Complete Init yoksa 401, varsa 501 döner. HTTP çağırmaz.
func Complete(text string) (string, error) {
	client, err := requireClient()
	if err != nil {
		return "", err
	}
	return client.Complete(text)
}

// CompleteFile Init yoksa 401, varsa 501 döner. HTTP çağırmaz.
func CompleteFile(path string) (string, error) {
	client, err := requireClient()
	if err != nil {
		return "", err
	}
	return client.CompleteFile(path)
}

// CompleteDocuments Init yoksa 401, varsa 501 döner. HTTP çağırmaz.
func CompleteDocuments(paths []string) (string, error) {
	client, err := requireClient()
	if err != nil {
		return "", err
	}
	return client.CompleteDocuments(paths)
}
