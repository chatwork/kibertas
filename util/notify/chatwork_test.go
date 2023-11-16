package notify

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestAddMessage(t *testing.T) {
	c := NewChatwork("token", "room", func() *logrus.Entry { return logrus.NewEntry(logrus.New()) })
	c.AddMessage("test")

	if c.Messages.String() != "test" {
		t.Errorf("add message failed: %s", c.Messages.String())
	}
}

func TestSend(t *testing.T) {
	c := NewChatwork("token", "room", func() *logrus.Entry { return logrus.NewEntry(logrus.New()) })
	c.AddMessage("test")
	c.Site = "localhost:8080"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected 'POST' request, got '%s'", r.Method)
		}

		if r.URL.Path != fmt.Sprintf("/v2/rooms/%s/messages", c.RoomId) {
			t.Errorf("Expected URL path '/v2/rooms/%s/messages', got '%s'", c.RoomId, r.URL.Path)
		}

		if r.Header.Get("X-ChatWorkToken") != c.ApiToken {
			t.Errorf("Expected 'X-ChatWorkToken' header to be '%s', got '%s'", c.ApiToken, r.Header.Get("X-ChatWorkToken"))
		}

		if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Errorf("Expected 'Content-Type' header to be 'application/x-www-form-urlencoded', got '%s'", r.Header.Get("Content-Type"))
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}

		if string(body) != "body=test" {
			t.Errorf("Expected request body 'body=test', got '%s'", string(body))
		}

		fmt.Fprintln(w, "OK")
	}))

	defer ts.Close()

	c.Site = ts.URL[7:]

	c.Send()
}
