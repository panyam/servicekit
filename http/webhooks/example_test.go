package webhooks_test

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/panyam/servicekit/http/webhooks"
)

func Example() {
	secret := webhooks.GenerateSecret()

	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		id, ts, sig := r.Header.Get(webhooks.HeaderID), r.Header.Get(webhooks.HeaderTimestamp), r.Header.Get(webhooks.HeaderSignature)
		if !webhooks.TimestampWithinTolerance(ts, time.Now(), webhooks.DefaultTolerance) ||
			!webhooks.Verify(secret, id, ts, body, sig) {
			http.Error(w, "bad signature", http.StatusUnauthorized)
			return
		}
		fmt.Printf("verified %s: %s\n", id, body)
	}))
	defer receiver.Close()

	send := func(body []byte, signWith string) int {
		msgID := "msg_1"
		ts := webhooks.Timestamp(time.Now())
		req, _ := http.NewRequest(http.MethodPost, receiver.URL, bytes.NewReader(body))
		req.Header.Set(webhooks.HeaderID, msgID)
		req.Header.Set(webhooks.HeaderTimestamp, ts)
		req.Header.Set(webhooks.HeaderSignature, webhooks.Sign(signWith, msgID, ts, body))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			panic(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	fmt.Println(send([]byte(`{"event":"created"}`), secret))
	fmt.Println(send([]byte(`{"event":"created"}`), webhooks.GenerateSecret()))
	// Output:
	// verified msg_1: {"event":"created"}
	// 200
	// 401
}
