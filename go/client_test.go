package bbmask

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func taggedBody(extra map[string]any) map[string]any {
	body := map[string]any{
		"doc_id":            "doc-1",
		"text":              "Davacı <PERSON_0>",
		"entities_found":    map[string]int{"PERSON": 1},
		"balance_credits":   "9.00",
		"state":             nil,
		"placeholder_style": "angle",
		"keep_legal_form":   false,
		"health_data":       false,
		"llm_review":        nil,
	}
	for key, value := range extra {
		body[key] = value
	}
	return body
}

func writeJSON(w http.ResponseWriter, status int, body any, requestID string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Request-Id", requestID)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func requireBB(t *testing.T, err error, status int) *BBError {
	t.Helper()
	if err == nil {
		t.Fatal("hata beklendi")
	}
	var bb *BBError
	if !errors.As(err, &bb) {
		t.Fatalf("tip %T: %v", err, err)
	}
	if bb.StatusCode != status {
		t.Fatalf("status %d detail %q", bb.StatusCode, bb.Detail)
	}
	return bb
}

func TestAnonymizeSendsWireBody(t *testing.T) {
	var seen struct {
		method string
		url    string
		auth   string
		body   map[string]any
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		seen.method = r.Method
		seen.url = r.URL.RequestURI()
		seen.auth = r.Header.Get("Authorization")
		_ = json.Unmarshal(raw, &seen.body)
		writeJSON(w, 200, taggedBody(nil), "req-1")
	}))
	defer server.Close()

	client := New(server.URL+"/", "dm_test_abc")
	tagged, err := client.Anonymize("Davacı Hasan Demirkol",
		WithPlaceholderStyle(StyleBracketTR),
		WithKeepLegalForm(true),
		WithHealthData(true),
		WithLLMReview(true),
	)
	if err != nil {
		t.Fatal(err)
	}
	if seen.method != http.MethodPost || seen.url != "/v1/anonymize" || seen.auth != "Bearer dm_test_abc" {
		t.Fatalf("istek %+v", seen)
	}
	want := map[string]any{
		"text":              "Davacı Hasan Demirkol",
		"persist":           true,
		"placeholder_style": "bracket_tr",
		"keep_legal_form":   true,
		"health_data":       true,
		"llm_review":        true,
	}
	if !reflect.DeepEqual(seen.body, want) {
		t.Fatalf("gövde %#v", seen.body)
	}
	if tagged.Text != "Davacı <PERSON_0>" || tagged.DocID != "doc-1" || tagged.State != nil {
		t.Fatalf("tagged %+v", tagged)
	}
	if !reflect.DeepEqual(tagged.EntitiesFound, map[string]int{"PERSON": 1}) {
		t.Fatalf("entities %#v", tagged.EntitiesFound)
	}
	if tagged.BalanceCredits != "9.00" || tagged.RequestID != "req-1" || tagged.PlaceholderStyle != "angle" {
		t.Fatalf("alanlar %+v", tagged)
	}
	if tagged.KeepLegalForm || tagged.HealthData || tagged.LLMReview != nil {
		t.Fatalf("seçenek alanları %+v", tagged)
	}
}

func TestPersistFalseDropsDocID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["persist"] != false {
			t.Fatalf("persist %#v", body["persist"])
		}
		writeJSON(w, 200, taggedBody(map[string]any{
			"state": map[string]any{"PERSON": map[string]any{"Ali": "<PERSON_0>"}},
		}), "req-1")
	}))
	defer server.Close()

	tagged, err := New(server.URL, "dm_test_abc").Anonymize("Ali", WithPersist(false))
	if err != nil {
		t.Fatal(err)
	}
	if tagged.DocID != "" {
		t.Fatalf("doc id %q", tagged.DocID)
	}
	want := map[string]any{"PERSON": map[string]any{"Ali": "<PERSON_0>"}}
	if !reflect.DeepEqual(tagged.State, want) {
		t.Fatalf("state %#v", tagged.State)
	}
}

func TestInvalidPlaceholderStyleFailsBeforeRequest(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		writeJSON(w, 200, taggedBody(nil), "req-1")
	}))
	defer server.Close()

	_, err := New(server.URL, "dm_test_abc").Anonymize("Ali", WithPlaceholderStyle("stars"))
	bb := requireBB(t, err, 400)
	if !strings.Contains(bb.Detail, "placeholder_style") {
		t.Fatalf("detail %q", bb.Detail)
	}
	if calls != 0 {
		t.Fatalf("çağrı %d", calls)
	}
}

func TestRestoreSendsDocIDOrState(t *testing.T) {
	var bodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		bodies = append(bodies, body)
		writeJSON(w, 200, map[string]any{"text": "açık metin"}, "req-1")
	}))
	defer server.Close()

	client := New(server.URL, "dm_test_abc")
	first, err := client.Restore("doc-1", "<PERSON_0>")
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.Restore(map[string]any{"PERSON": map[string]any{}}, "<PERSON_0>")
	if err != nil {
		t.Fatal(err)
	}
	if first != "açık metin" || second != "açık metin" {
		t.Fatalf("%q %q", first, second)
	}
	if !reflect.DeepEqual(bodies[0], map[string]any{"text": "<PERSON_0>", "doc_id": "doc-1"}) {
		t.Fatalf("doc %#v", bodies[0])
	}
	if !reflect.DeepEqual(bodies[1], map[string]any{"text": "<PERSON_0>", "state": map[string]any{"PERSON": map[string]any{}}}) {
		t.Fatalf("state %#v", bodies[1])
	}
	_, err = client.Restore(12, "x")
	requireBB(t, err, 400)
}

func TestGetMapEncodesIDAndDeleteReadsDeleted(t *testing.T) {
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.RequestURI())
		if r.Method == http.MethodGet {
			writeJSON(w, 200, map[string]any{"state": map[string]any{"PERSON": map[string]any{"Ali": "<PERSON_0>"}}}, "req-1")
			return
		}
		writeJSON(w, 200, map[string]any{"deleted": true}, "req-1")
	}))
	defer server.Close()

	client := New(server.URL, "dm_test_abc")
	state, err := client.GetMap("a/b c", true)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"PERSON": map[string]any{"Ali": "<PERSON_0>"}}
	if !reflect.DeepEqual(state, want) {
		t.Fatalf("state %#v", state)
	}
	deleted, err := client.Delete("a/b c")
	if err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("deleted false")
	}
	if seen[0] != "GET /v1/mappings/a%2Fb%20c?once=true" {
		t.Fatalf("get %s", seen[0])
	}
	if seen[1] != "DELETE /v1/mappings/a%2Fb%20c" {
		t.Fatalf("delete %s", seen[1])
	}
}

func TestFileUploadUsesMultipartFieldNames(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dilekce.txt")
	if err := os.WriteFile(path, []byte("Davacı Ali"), 0o644); err != nil {
		t.Fatal(err)
	}
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		if r.URL.Path != "/v1/anonymize/file" {
			t.Fatalf("yol %s", r.URL.Path)
		}
		writeJSON(w, 200, taggedBody(map[string]any{"doc_id": "should-drop"}), "req-1")
	}))
	defer server.Close()

	tagged, err := New(server.URL, "dm_test_abc").AnonymizeFile(path, WithPersist(false))
	if err != nil {
		t.Fatal(err)
	}
	if tagged.DocID != "" {
		t.Fatalf("doc id %q", tagged.DocID)
	}
	for _, part := range []string{
		"name=\"persist\"\r\n\r\nfalse",
		"name=\"placeholder_style\"\r\n\r\nangle",
		"name=\"keep_legal_form\"\r\n\r\nfalse",
		"name=\"health_data\"\r\n\r\nfalse",
		"name=\"llm_review\"\r\n\r\nfalse",
		"name=\"file\"",
		"filename=\"dilekce.txt\"",
		"Davacı Ali",
	} {
		if !strings.Contains(body, part) {
			t.Fatalf("eksik %q\n%s", part, body)
		}
	}
}

func TestDocumentsCount(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "a.txt")
	second := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(first, []byte("bir"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("iki"), 0o644); err != nil {
		t.Fatal(err)
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		raw, _ := io.ReadAll(r.Body)
		body := string(raw)
		if r.URL.Path != "/v1/anonymize/documents" {
			t.Fatalf("yol %s", r.URL.Path)
		}
		if strings.Count(body, "name=\"files\"") != 2 || !strings.Contains(body, "filename=\"a.txt\"") || !strings.Contains(body, "filename=\"b.txt\"") {
			t.Fatalf("gövde %s", body)
		}
		writeJSON(w, 200, taggedBody(nil), "req-1")
	}))
	defer server.Close()

	client := New(server.URL, "dm_test_abc")
	tagged, err := client.AnonymizeDocuments([]string{first, second})
	if err != nil {
		t.Fatal(err)
	}
	if tagged.DocID != "doc-1" {
		t.Fatalf("doc id %q", tagged.DocID)
	}
	_, err = client.AnonymizeDocuments(nil)
	requireBB(t, err, 400)
	_, err = client.AnonymizeDocuments([]string{"1", "2", "3", "4"}, WithPlaceholderStyle("nope"))
	bb := requireBB(t, err, 400)
	if !strings.Contains(bb.Detail, "placeholder_style") {
		t.Fatalf("detail %q", bb.Detail)
	}
	if calls != 1 {
		t.Fatalf("çağrı %d", calls)
	}
}

func TestHTTPErrorKeepsStatusDetailAndRequestID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 402, map[string]any{"detail": "Bakiye yok"}, "req-402")
	}))
	defer server.Close()

	_, err := New(server.URL, "dm_test_abc").Anonymize("Ali")
	bb := requireBB(t, err, 402)
	if bb.Detail != "Bakiye yok" || bb.RequestID != "req-402" || bb.Error() != "Bakiye yok" {
		t.Fatalf("%+v", bb)
	}
}

func TestNonJSONSuccessIs502(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "req-bad")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("tamam"))
	}))
	defer server.Close()

	_, err := New(server.URL, "dm_test_abc").Anonymize("Ali")
	bb := requireBB(t, err, 502)
	if bb.RequestID != "req-bad" || bb.Detail != "Servis JSON dönmedi" {
		t.Fatalf("%+v", bb)
	}
}

func TestTimeoutIs504AndClosedPortIs503(t *testing.T) {
	release := make(chan struct{})
	hanging := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	t.Cleanup(func() {
		close(release)
		hanging.Close()
	})

	_, err := New(hanging.URL, "dm_test_abc", 200*time.Millisecond).Anonymize("Ali")
	requireBB(t, err, 504)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	_, err = New("http://"+addr, "dm_test_abc", time.Second).Anonymize("Ali")
	requireBB(t, err, 503)
}

func TestAnonymizeBytesSendsNameAndContent(t *testing.T) {
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		writeJSON(w, 200, taggedBody(nil), "req-1")
	}))
	defer server.Close()

	tagged, err := New(server.URL, "dm_test_abc").AnonymizeBytes("ek.pdf", []byte("%PDF"))
	if err != nil {
		t.Fatal(err)
	}
	if tagged.DocID != "doc-1" || !strings.Contains(body, `filename="ek.pdf"`) || !strings.Contains(body, "%PDF") {
		t.Fatalf("gövde %q", body)
	}
}

func TestTokenIsNotPrinted(t *testing.T) {
	client := New("http://127.0.0.1:9", "dm_test_secret")
	if strings.Contains(fmt.Sprintf("%+v", *client), "dm_test_secret") {
		t.Fatal("anahtar yazdırıldı")
	}
}

func TestSessionDeletesIDsAndHidesState(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.RequestURI())
		if r.Method == http.MethodDelete {
			if strings.HasSuffix(r.URL.Path, "id-2") {
				writeJSON(w, 404, map[string]any{"detail": "yok"}, "req-1")
				return
			}
			writeJSON(w, 200, map[string]any{"deleted": true}, "req-1")
			return
		}
		var incoming map[string]any
		_ = json.NewDecoder(r.Body).Decode(&incoming)
		if r.URL.Path == "/v1/restore" {
			writeJSON(w, 200, map[string]any{"text": "<PERSON_0>"}, "req-1")
			return
		}
		persist, _ := incoming["persist"].(bool)
		body := taggedBody(map[string]any{"text": "<PERSON_0>"})
		if persist {
			body["doc_id"] = "id-" + strconv.Itoa(len(calls))
			body["state"] = nil
		} else {
			body["doc_id"] = "should-drop"
			body["state"] = map[string]any{"PERSON": map[string]any{"Ali": "<PERSON_0>"}}
		}
		writeJSON(w, 200, body, "req-1")
	}))
	defer server.Close()

	if _, err := Init("dm_test_abc", Config{BaseURL: server.URL}); err != nil {
		t.Fatal(err)
	}
	open, err := Session()
	if err != nil {
		t.Fatal(err)
	}
	first, err := open.Anonymize("Ali")
	if err != nil {
		t.Fatal(err)
	}
	second, err := open.Anonymize("Veli")
	if err != nil {
		t.Fatal(err)
	}
	restored, err := open.Restore("cevap")
	if err != nil {
		t.Fatal(err)
	}
	if restored != "<PERSON_0>" {
		t.Fatalf("restore %q", restored)
	}
	if err := open.Close(); err != nil {
		t.Fatal(err)
	}
	if !contains(calls, "DELETE /v1/mappings/"+first.DocID) || !contains(calls, "DELETE /v1/mappings/"+second.DocID) {
		t.Fatalf("silme %v", calls)
	}

	local, err := Session(WithPersist(false), WithPlaceholderStyle(StyleBracketTR))
	if err != nil {
		t.Fatal(err)
	}
	tagged, err := local.Anonymize("Ali")
	if err != nil {
		t.Fatal(err)
	}
	if tagged.DocID != "" {
		t.Fatalf("doc id %q", tagged.DocID)
	}
	if _, err := local.Restore("<PERSON_0>"); err != nil {
		t.Fatal(err)
	}
	before := len(calls)
	if err := local.Close(); err != nil {
		t.Fatal(err)
	}
	if len(calls) != before {
		t.Fatalf("persist false silme gitti: %v", calls[before:])
	}
	_, err = local.Restore("tekrar")
	requireBB(t, err, 400)

	empty, err := Session()
	if err != nil {
		t.Fatal(err)
	}
	_, err = empty.Restore("yok")
	requireBB(t, err, 400)

	_, err = Session(WithPlaceholderStyle("nope"))
	requireBB(t, err, 400)
}

func TestModuleCallsFollowInit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ek.txt")
	if err := os.WriteFile(path, []byte("ek"), 0o644); err != nil {
		t.Fatal(err)
	}
	var seen []string
	var auth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.RequestURI())
		if auth == "" {
			auth = r.Header.Get("Authorization")
		}
		if r.Method == http.MethodPost && r.URL.Path == "/v1/restore" {
			writeJSON(w, 200, map[string]any{"text": "ok"}, "req-1")
			return
		}
		writeJSON(w, 200, taggedBody(nil), "req-1")
	}))
	defer server.Close()

	_, err := Init("  ")
	requireBB(t, err, 401)
	if _, err := Init("  dm_test_abc  ", Config{BaseURL: server.URL}); err != nil {
		t.Fatal(err)
	}
	tagged, err := Anonymize("Ali")
	if err != nil {
		t.Fatal(err)
	}
	if tagged.DocID != "doc-1" {
		t.Fatalf("doc id %q", tagged.DocID)
	}
	text, err := Restore(tagged.DocID, "cevap")
	if err != nil {
		t.Fatal(err)
	}
	if text != "ok" {
		t.Fatalf("restore %q", text)
	}
	state, err := GetMap(tagged.DocID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(state) != 0 {
		t.Fatalf("state %#v", state)
	}
	deleted, err := Delete(tagged.DocID)
	if err != nil {
		t.Fatal(err)
	}
	if deleted {
		t.Fatal("deleted true")
	}
	if _, err := AnonymizeFile(path); err != nil {
		t.Fatal(err)
	}
	if _, err := AnonymizeBytes("ek.txt", []byte("ek")); err != nil {
		t.Fatal(err)
	}
	if _, err := AnonymizeDocuments([]string{path}); err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer dm_test_abc" {
		t.Fatalf("auth %q", auth)
	}
	want := []string{
		"/v1/anonymize",
		"/v1/restore",
		"/v1/mappings/doc-1?once=false",
		"/v1/mappings/doc-1",
		"/v1/anonymize/file",
		"/v1/anonymize/file",
		"/v1/anonymize/documents",
	}
	if !reflect.DeepEqual(seen, want) {
		t.Fatalf("yollar %#v", seen)
	}
}

func TestModuleCallsBeforeInit(t *testing.T) {
	mu.Lock()
	previous := current
	current = nil
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		current = previous
		mu.Unlock()
	})

	_, err := Anonymize("Ali")
	requireBB(t, err, 401)
	_, err = AnonymizeBytes("a.txt", []byte("a"))
	requireBB(t, err, 401)
	_, err = Session()
	requireBB(t, err, 401)
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
