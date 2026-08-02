package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func jsonResponse(status int, body any) *http.Response {
	var text string
	if s, ok := body.(string); ok {
		text = s
	} else {
		b, _ := json.Marshal(body)
		text = string(b)
	}
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(text)),
		Header:     make(http.Header),
	}
}

func testClient(t *testing.T, token string, rt roundTripFunc) *HTTPClient {
	t.Helper()
	if token == "" {
		token = "secret-token-123"
	}
	return NewWithTransport(Options{BaseURL: "http://localhost:41184", Token: token}, rt)
}

func TestURLConstruction(t *testing.T) {
	ctx := context.Background()

	t.Run("encodes an ID containing special characters into the URL path", func(t *testing.T) {
		var captured *http.Request
		c := testClient(t, "", func(req *http.Request) (*http.Response, error) {
			captured = req
			return jsonResponse(200, map[string]any{"id": "x", "title": "y"}), nil
		})

		if _, err := c.GetNote(ctx, "abc/def&ghi", nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := captured.URL.String()
		if strings.Contains(got, "abc/def&ghi") {
			t.Errorf("URL %q contains the raw unencoded ID", got)
		}
		if !strings.Contains(got, url.QueryEscape("abc/def&ghi")) {
			t.Errorf("URL %q does not contain the encoded ID", got)
		}
		if !strings.HasPrefix(got, "http://localhost:41184/notes/abc%2Fdef%26ghi?") {
			t.Errorf("URL %q does not have the expected prefix", got)
		}
	})

	t.Run("encodes both IDs in the two-ID RemoveTagFromNote path", func(t *testing.T) {
		var captured *http.Request
		c := testClient(t, "", func(req *http.Request) (*http.Response, error) {
			captured = req
			return jsonResponse(200, map[string]any{}), nil
		})

		if err := c.RemoveTagFromNote(ctx, "tag/1", "note&2"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := "/tags/" + url.QueryEscape("tag/1") + "/notes/" + url.QueryEscape("note&2")
		if !strings.Contains(captured.URL.String(), want) {
			t.Errorf("URL %q does not contain %q", captured.URL.String(), want)
		}
	})

	t.Run("appends the token as an encoded query param and never leaks it into a thrown error message", func(t *testing.T) {
		var captured *http.Request
		c := testClient(t, "super-secret-token", func(req *http.Request) (*http.Response, error) {
			captured = req
			return jsonResponse(404, "not found"), nil
		})

		_, err := c.GetNote(ctx, "n1", nil)

		if !strings.Contains(captured.URL.String(), "token="+url.QueryEscape("super-secret-token")) {
			t.Errorf("URL %q does not contain the encoded token", captured.URL.String())
		}

		var apiErr *APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("got error %v (%T), want *APIError", err, err)
		}
		if apiErr.Status != 404 {
			t.Errorf("status = %d, want 404", apiErr.Status)
		}
		if strings.Contains(apiErr.Message, "super-secret-token") {
			t.Errorf("error message %q leaks the token", apiErr.Message)
		}
	})

	t.Run("never leaks the token into a network-failure error message either", func(t *testing.T) {
		// http.Client.Do always wraps whatever error a RoundTripper returns in
		// its OWN *url.Error, whose Error() string embeds the full request URL
		// (including ?token=...) — the RoundTripper stub must return the bare
		// underlying cause here, not pre-wrap it itself, or this test would
		// exercise a double-wrapped error that doesn't reflect a real failure.
		c := testClient(t, "super-secret-token", func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("connection refused")
		})

		_, err := c.GetNote(ctx, "n1", nil)
		if err == nil {
			t.Fatal("expected an error")
		}
		if strings.Contains(err.Error(), "super-secret-token") {
			t.Errorf("error message %q leaks the token", err.Error())
		}
		if !strings.Contains(err.Error(), "connection refused") {
			t.Errorf("error message %q lost the underlying cause", err.Error())
		}
	})

	t.Run("sends a JSON body with Content-Type only when a body is present", func(t *testing.T) {
		var captured *http.Request
		var capturedBody []byte
		c := testClient(t, "", func(req *http.Request) (*http.Response, error) {
			captured = req
			if req.Body != nil {
				capturedBody, _ = io.ReadAll(req.Body)
			}
			return jsonResponse(200, map[string]any{"id": "n1"}), nil
		})

		if _, err := c.CreateNote(ctx, map[string]any{"title": "hi"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if captured.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", captured.Method)
		}
		if got := captured.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}
		if string(capturedBody) != `{"title":"hi"}` {
			t.Errorf("body = %s, want {\"title\":\"hi\"}", capturedBody)
		}

		if err := c.DeleteNote(ctx, "n1"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if captured.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", captured.Method)
		}
		if got := captured.Header.Get("Content-Type"); got != "" {
			t.Errorf("Content-Type = %q, want unset for a bodyless request", got)
		}
	})

	t.Run("sends resource creation as multipart with a boundary Content-Type", func(t *testing.T) {
		var captured *http.Request
		var capturedBody []byte
		c := testClient(t, "", func(req *http.Request) (*http.Response, error) {
			captured = req
			capturedBody, _ = io.ReadAll(req.Body)
			return jsonResponse(200, map[string]any{"id": "res1", "title": "photo.png"}), nil
		})

		if _, err := c.CreateResource(ctx, []byte{1, 2, 3}, "photo.png", "image/png"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(captured.URL.String(), "/resources") {
			t.Errorf("URL %q does not contain /resources", captured.URL.String())
		}
		if captured.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", captured.Method)
		}
		ct := captured.Header.Get("Content-Type")
		if !strings.HasPrefix(ct, "multipart/form-data; boundary=") {
			t.Errorf("Content-Type = %q, want a multipart/form-data boundary", ct)
		}
		if !strings.Contains(string(capturedBody), `name="props"`) || !strings.Contains(string(capturedBody), `"title":"photo.png"`) {
			t.Errorf("body %q does not contain the expected props field", capturedBody)
		}
	})

	t.Run("restores a note/notebook via PUT with deleted_time: 0", func(t *testing.T) {
		var captured *http.Request
		var capturedBody []byte
		c := testClient(t, "", func(req *http.Request) (*http.Response, error) {
			captured = req
			capturedBody, _ = io.ReadAll(req.Body)
			return jsonResponse(200, map[string]any{}), nil
		})

		if err := c.RestoreNote(ctx, "n1"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(captured.URL.String(), "/notes/n1") || captured.Method != http.MethodPut {
			t.Errorf("expected PUT /notes/n1, got %s %s", captured.Method, captured.URL.String())
		}
		if string(capturedBody) != `{"deleted_time":0}` {
			t.Errorf("body = %s, want {\"deleted_time\":0}", capturedBody)
		}

		if err := c.RestoreNotebook(ctx, "nb1"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(captured.URL.String(), "/folders/nb1") || captured.Method != http.MethodPut {
			t.Errorf("expected PUT /folders/nb1, got %s %s", captured.Method, captured.URL.String())
		}
		if string(capturedBody) != `{"deleted_time":0}` {
			t.Errorf("body = %s, want {\"deleted_time\":0}", capturedBody)
		}
	})

	t.Run("wraps a network failure in an APIError and Ping reports unreachable rather than erroring", func(t *testing.T) {
		c := testClient(t, "", func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("ECONNREFUSED")
		})

		if c.Ping(ctx) {
			t.Error("Ping() = true, want false")
		}

		_, err := c.GetNote(ctx, "n1", nil)
		if err == nil || !strings.Contains(err.Error(), "cannot reach Joplin") {
			t.Errorf("got error %v, want one containing %q", err, "cannot reach Joplin")
		}
	})
}

func TestPagination(t *testing.T) {
	ctx := context.Background()

	t.Run("follows has_more across pages and stops when it goes false", func(t *testing.T) {
		calls := 0
		var urls []string
		c := testClient(t, "", func(req *http.Request) (*http.Response, error) {
			calls++
			urls = append(urls, req.URL.String())
			if calls == 1 {
				return jsonResponse(200, map[string]any{
					"items":    []any{map[string]any{"id": "a"}, map[string]any{"id": "b"}},
					"has_more": true,
				}), nil
			}
			return jsonResponse(200, map[string]any{
				"items":    []any{map[string]any{"id": "c"}},
				"has_more": false,
			}), nil
		})

		result, err := c.ListNotebooks(ctx, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 3 || result[0]["id"] != "a" || result[1]["id"] != "b" || result[2]["id"] != "c" {
			t.Errorf("got %v, want [a b c]", result)
		}
		if calls != 2 {
			t.Errorf("calls = %d, want 2", calls)
		}
		if !strings.Contains(urls[0], "page=1") || !strings.Contains(urls[1], "page=2") {
			t.Errorf("urls = %v, want page=1 then page=2", urls)
		}
	})

	t.Run("stops paging when a page comes back empty even if has_more claims true", func(t *testing.T) {
		calls := 0
		c := testClient(t, "", func(req *http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				return jsonResponse(200, map[string]any{"items": []any{map[string]any{"id": "a"}}, "has_more": true}), nil
			}
			return jsonResponse(200, map[string]any{"items": []any{}, "has_more": true}), nil
		})

		result, err := c.ListNotebooks(ctx, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 1 || result[0]["id"] != "a" {
			t.Errorf("got %v, want [a]", result)
		}
		if calls != 2 {
			t.Errorf("calls = %d, want 2", calls)
		}
	})

	t.Run("respects a finite limit and slices the accumulated results", func(t *testing.T) {
		c := testClient(t, "", func(req *http.Request) (*http.Response, error) {
			return jsonResponse(200, map[string]any{
				"items":    []any{map[string]any{"id": "a"}, map[string]any{"id": "b"}},
				"has_more": true,
			}), nil
		})

		result, err := c.ListNotes(ctx, ListNotesOptions{Limit: 1})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 1 || result[0]["id"] != "a" {
			t.Errorf("got %v, want [a]", result)
		}
	})
}
