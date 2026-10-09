package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newTest(h http.HandlerFunc) (*Client, *[]time.Duration) {
	srv := httptest.NewServer(h)
	c := New(srv.URL, "hst_live_x", "test/1")
	var slept []time.Duration
	c.Sleep = func(_ context.Context, d time.Duration) error { slept = append(slept, d); return nil }
	return c, &slept
}

func TestRetryHonoursRetryAfterOn429(t *testing.T) {
	var n int32
	c, slept := newTest(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&n, 1) < 3 {
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(429)
			_, _ = w.Write([]byte(`{"error":{"code":"rate_limit_exceeded","message":"x","status":429}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"r1","slug":"tlv-dc-1"}]}`))
	})
	regions, err := c.ListRegions(context.Background())
	if err != nil || len(regions) != 1 {
		t.Fatalf("got %v, %v", regions, err)
	}
	if len(*slept) != 2 || (*slept)[0] != 7*time.Second {
		t.Fatalf("sleeps = %v, want two 7s waits", *slept)
	}
}

func TestNoRetryForNonIdempotentPOSTOn503(t *testing.T) {
	var n int32
	c, _ := newTest(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		w.WriteHeader(503)
	})
	if _, err := c.InstanceAction(context.Background(), "i1", "start"); err == nil {
		t.Fatal("expected error")
	}
	if n != 1 {
		t.Fatalf("POST without Idempotency-Key retried %d times", n)
	}
}

func TestIdempotentCreateRetriedOn503(t *testing.T) {
	var n int32
	var keys []string
	c, _ := newTest(func(w http.ResponseWriter, r *http.Request) {
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		if atomic.AddInt32(&n, 1) == 1 {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(202)
		_, _ = w.Write([]byte(`{"data":{"id":12,"status":"pending","instance_id":"u-1"}}`))
	})
	op, err := c.CreateInstance(context.Background(), CreateInstanceRequest{PlanID: "p"}, "k-1")
	if err != nil || op.InstanceID != "u-1" || op.ID != "12" {
		t.Fatalf("op=%+v err=%v", op, err)
	}
	if len(keys) != 2 || keys[0] != "k-1" || keys[1] != "k-1" {
		t.Fatalf("idempotency keys %v", keys)
	}
}

func TestErrorDecodingAndKeyNotInError(t *testing.T) {
	c, _ := newTest(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(409)
		_, _ = w.Write([]byte(`{"error":{"code":"deletion_protected","message":"protected","status":409},"meta":{"request_id":"abc"}}`))
	})
	_, err := c.DeleteInstance(context.Background(), "i1", "web")
	if !HasCode(err, "deletion_protected") {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "hst_live_x") {
		t.Fatal("API key leaked into an error message")
	}
	if !strings.Contains(err.Error(), "request_id abc") {
		t.Fatalf("request id missing: %v", err)
	}
}

func TestWaitOperationFails(t *testing.T) {
	c, _ := newTest(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"id":5,"type":"resize","status":"error","failure_reason":"no capacity"}}`))
	})
	_, err := c.WaitOperation(context.Background(), &Operation{ID: "5", Status: "pending"})
	if err == nil || !strings.Contains(err.Error(), "no capacity") {
		t.Fatalf("err = %v", err)
	}
}
