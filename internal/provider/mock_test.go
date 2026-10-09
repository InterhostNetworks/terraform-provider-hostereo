package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const testAPIKey = "hst_live_TESTKEY_never_log_me_0123456789"

// mockAPI is a small in-memory Hostereo External API.
type mockAPI struct {
	t   *testing.T
	srv *httptest.Server
	mu  sync.Mutex

	instances map[string]*mockInstance
	ops       map[string]*mockOp
	projects  map[string]map[string]any
	tags      map[string]map[string]any
	nextID    int

	// recorded traffic
	Calls        []string // "METHOD /path"
	CreateBodies []map[string]any
	IdemKeys     []string
	DeleteBodies []map[string]any
	AuthHeaders  []string
	UserAgents   []string

	// knobs
	ProtectNew   bool // deletion_protection for new instances
	RateLimitNth int  // answer 429 to this many /regions calls first
}

type mockInstance struct {
	ID, Name, Status, PlanID, Region string
	Protected                        bool
	ProjectID                        string
	TagIDs                           []string
	Deleted                          bool
}

type mockOp struct {
	ID, Type, InstanceID string
	Polls                int
	apply                func()
}

func newMockAPI(t *testing.T) *mockAPI {
	m := &mockAPI{t: t, instances: map[string]*mockInstance{}, ops: map[string]*mockOp{}, projects: map[string]map[string]any{}, tags: map[string]map[string]any{}, ProtectNew: true}
	m.srv = httptest.NewServer(http.HandlerFunc(m.handle))
	t.Cleanup(m.srv.Close)
	return m
}

func (m *mockAPI) Endpoint() string { return m.srv.URL + "/api/v1" }

func (m *mockAPI) uuid() string {
	m.nextID++
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", m.nextID)
}

var plans = map[string]map[string]any{
	"11111111-1111-4111-8111-111111111111": {"id": "11111111-1111-4111-8111-111111111111", "name": "Basic", "cpu": 1, "memory_mb": 2048, "disk_gb": 25},
	"22222222-2222-4222-8222-222222222222": {"id": "22222222-2222-4222-8222-222222222222", "name": "Pro", "cpu": 4, "memory_mb": 8192, "disk_gb": 100},
}

func (m *mockAPI) instanceJSON(i *mockInstance) map[string]any {
	return map[string]any{
		"id": i.ID, "name": i.Name, "status": i.Status, "locked": false, "created_at": "2026-10-09T10:00:00+00:00",
		"region":              map[string]any{"id": "33333333-3333-4333-8333-333333333333", "name": "Tel Aviv", "slug": "tlv-dc-1"},
		"plan":                plans[i.PlanID],
		"deletion_protection": i.Protected,
	}
}

func (m *mockAPI) newOp(typ, instanceID string, apply func()) *mockOp {
	op := &mockOp{ID: fmt.Sprint(len(m.ops) + 100), Type: typ, InstanceID: instanceID, apply: apply}
	m.ops[op.ID] = op
	return op
}

func (m *mockAPI) opJSON(op *mockOp, status string) map[string]any {
	return map[string]any{"id": json.Number(op.ID), "type": op.Type, "status": status, "instance_id": op.InstanceID, "result": nil}
}

func write(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"data": data, "meta": map[string]any{"request_id": "test"}})
}

func writeErr(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": code, "message": msg, "status": status}, "meta": map[string]any{"request_id": "test"}})
}

func (m *mockAPI) handle(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := strings.TrimPrefix(r.URL.Path, "/api/v1")
	m.Calls = append(m.Calls, r.Method+" "+p)
	m.AuthHeaders = append(m.AuthHeaders, r.Header.Get("Authorization"))
	m.UserAgents = append(m.UserAgents, r.Header.Get("User-Agent"))
	var body map[string]any
	if raw, _ := io.ReadAll(r.Body); len(raw) > 0 {
		_ = json.Unmarshal(raw, &body)
	}
	seg := strings.Split(strings.Trim(p, "/"), "/")

	// public catalogue
	switch p {
	case "/regions":
		if m.RateLimitNth > 0 {
			m.RateLimitNth--
			w.Header().Set("Retry-After", "1")
			writeErr(w, 429, "rate_limit_exceeded", "slow down")
			return
		}
		write(w, 200, []any{map[string]any{"id": "33333333-3333-4333-8333-333333333333", "name": "Tel Aviv", "slug": "tlv-dc-1", "active": true}})
		return
	case "/plans":
		out := []any{}
		for _, id := range []string{"11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"} {
			pl := map[string]any{}
			for k, v := range plans[id] {
				pl[k] = v
			}
			pl["type"] = "default"
			pl["pricing"] = map[string]any{"postpaid_monthly": 10.5, "prepaid_monthly": 10, "prepaid_annual": 100}
			out = append(out, pl)
		}
		write(w, 200, out)
		return
	case "/images":
		write(w, 200, []any{map[string]any{"id": "44444444-4444-4444-8444-444444444444", "os": "Ubuntu", "version": "24.04", "distribution": "Ubuntu 24.04", "datacenter_id": "33333333-3333-4333-8333-333333333333"}})
		return
	case "/applications":
		write(w, 200, []any{map[string]any{"id": 7, "name": "WordPress", "versions": []any{map[string]any{"marketplace_version_id": "55555555-5555-4555-8555-555555555555", "version": "6", "os": "Ubuntu 24.04", "datacenters": []any{map[string]any{"id": "33333333-3333-4333-8333-333333333333", "name": "tlv-dc-1"}}}}, "fields": []any{map[string]any{"name": "site_title", "type": "string", "required": true}}}})
		return
	}

	if r.Header.Get("Authorization") != "Bearer "+testAPIKey {
		writeErr(w, 401, "unauthenticated", "bad key")
		return
	}

	switch {
	case p == "/instances" && r.Method == http.MethodPost:
		m.CreateBodies = append(m.CreateBodies, body)
		m.IdemKeys = append(m.IdemKeys, r.Header.Get("Idempotency-Key"))
		inst := &mockInstance{ID: m.uuid(), Name: fmt.Sprint(body["name"]), Status: "processing", PlanID: fmt.Sprint(body["plan_id"]), Region: fmt.Sprint(body["region"]), Protected: m.ProtectNew}
		m.instances[inst.ID] = inst
		op := m.newOp("create", inst.ID, func() { inst.Status = "available" })
		write(w, 202, m.opJSON(op, "pending"))
	case p == "/instances" && r.Method == http.MethodGet:
		out := []any{}
		for _, i := range m.instances {
			if !i.Deleted {
				out = append(out, m.instanceJSON(i))
			}
		}
		write(w, 200, out)
	case len(seg) >= 2 && seg[0] == "instances":
		i := m.instances[seg[1]]
		if i == nil || i.Deleted {
			writeErr(w, 404, "instance_not_found", "no such instance")
			return
		}
		action := ""
		if len(seg) == 3 {
			action = seg[2]
		}
		switch {
		case action == "" && r.Method == http.MethodGet:
			write(w, 200, m.instanceJSON(i))
		case action == "" && r.Method == http.MethodDelete:
			m.DeleteBodies = append(m.DeleteBodies, body)
			if i.Protected {
				writeErr(w, 409, "deletion_protected", "Deletion protection is enabled for this instance.")
				return
			}
			if body["confirm"] != i.Name {
				writeErr(w, 422, "validation_error", "confirm must equal the instance name")
				return
			}
			op := m.newOp("delete", i.ID, func() { i.Deleted = true })
			write(w, 202, m.opJSON(op, "pending"))
		case action == "status":
			st := "running"
			if i.Status == "stopped" {
				st = "stopped"
			}
			write(w, 200, map[string]any{"instance_id": i.ID, "status": st})
		case action == "organisation":
			write(w, 200, m.org(i))
		case action == "project" && r.Method == http.MethodPut:
			if v, ok := body["project_id"].(string); ok {
				i.ProjectID = v
			} else {
				i.ProjectID = ""
			}
			write(w, 200, m.org(i))
		case action == "tags" && r.Method == http.MethodPut:
			i.TagIDs = nil
			for _, t := range body["tag_ids"].([]any) {
				i.TagIDs = append(i.TagIDs, t.(string))
			}
			write(w, 200, m.org(i))
		case action == "resize":
			target := fmt.Sprint(body["plan_id"])
			op := m.newOp("resize", i.ID, func() { i.PlanID = target })
			write(w, 202, m.opJSON(op, "pending"))
		case action == "stop" || action == "start":
			if (action == "stop") == (i.Status == "stopped") {
				writeErr(w, 409, "invalid_state", "already")
				return
			}
			op := m.newOp(action, i.ID, func() {
				if action == "stop" {
					i.Status = "stopped"
				} else {
					i.Status = "available"
				}
			})
			write(w, 202, m.opJSON(op, "pending"))
		default:
			writeErr(w, 405, "method_not_allowed", r.Method+" "+p)
		}
	case len(seg) == 2 && seg[0] == "operations":
		op := m.ops[seg[1]]
		if op == nil {
			writeErr(w, 404, "operation_not_found", "no op")
			return
		}
		op.Polls++
		if op.Polls < 2 {
			w.Header().Set("Retry-After", "1")
			write(w, 200, m.opJSON(op, "processing"))
			return
		}
		if op.apply != nil {
			op.apply()
			op.apply = nil
		}
		write(w, 200, m.opJSON(op, "success"))
	case seg[0] == "projects" || seg[0] == "tags":
		store := m.projects
		if seg[0] == "tags" {
			store = m.tags
		}
		switch {
		case len(seg) == 1 && r.Method == http.MethodPost:
			id := m.uuid()
			obj := map[string]any{"id": id, "name": body["name"], "instances_count": 0}
			if seg[0] == "projects" {
				obj["description"] = body["description"]
			}
			store[id] = obj
			write(w, 201, obj)
		case len(seg) == 2:
			obj := store[seg[1]]
			if obj == nil {
				writeErr(w, 404, "not_found", "nope")
				return
			}
			if r.Method == http.MethodPatch {
				for k, v := range body {
					obj[k] = v
				}
			}
			write(w, 200, obj)
		default:
			writeErr(w, 405, "method_not_allowed", p)
		}
	default:
		writeErr(w, 404, "not_found", p)
	}
}

func (m *mockAPI) org(i *mockInstance) map[string]any {
	var project any
	if i.ProjectID != "" {
		project = m.projects[i.ProjectID]
	}
	tags := []any{}
	for _, t := range i.TagIDs {
		tags = append(tags, map[string]any{"id": t, "name": "t"})
	}
	return map[string]any{"instance_id": i.ID, "project": project, "tags": tags}
}

func (m *mockAPI) count(prefix string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, c := range m.Calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}
