package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// FlexString decodes a JSON string or number (operation ids are integers
// today and may become UUIDs).
type FlexString string

func (f *FlexString) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" || s == "" {
		*f = ""
		return nil
	}
	if strings.HasPrefix(s, `"`) {
		var v string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		*f = FlexString(v)
		return nil
	}
	*f = FlexString(s)
	return nil
}

type Region struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Slug   string `json:"slug"`
	Active bool   `json:"active"`
}

type Pricing struct {
	PostpaidMonthly float64 `json:"postpaid_monthly"`
	PrepaidMonthly  float64 `json:"prepaid_monthly"`
	PrepaidAnnual   float64 `json:"prepaid_annual"`
}

type Plan struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	CPU      int64   `json:"cpu"`
	MemoryMB int64   `json:"memory_mb"`
	DiskGB   int64   `json:"disk_gb"`
	Pricing  Pricing `json:"pricing"`
}

type Image struct {
	ID           string `json:"id"`
	OS           string `json:"os"`
	Version      string `json:"version"`
	Distribution string `json:"distribution"`
	DatacenterID string `json:"datacenter_id"`
}

type ApplicationVersion struct {
	MarketplaceVersionID string `json:"marketplace_version_id"`
	Version              string `json:"version"`
	OS                   string `json:"os"`
	Datacenters          []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"datacenters"`
}

type Application struct {
	ID       FlexString           `json:"id"`
	Name     string               `json:"name"`
	Versions []ApplicationVersion `json:"versions"`
	Fields   []struct {
		Name        string `json:"name"`
		Type        string `json:"type"`
		Required    bool   `json:"required"`
		Description string `json:"description"`
	} `json:"fields"`
}

type InstancePlan struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	CPU      int64  `json:"cpu"`
	MemoryMB int64  `json:"memory_mb"`
	DiskGB   int64  `json:"disk_gb"`
}

type Instance struct {
	ID                 string        `json:"id"`
	Name               string        `json:"name"`
	Status             string        `json:"status"`
	Region             *Region       `json:"region"`
	Plan               *InstancePlan `json:"plan"`
	Locked             bool          `json:"locked"`
	CreatedAt          string        `json:"created_at"`
	DeletionProtection *bool         `json:"deletion_protection"`
}

type InstanceStatus struct {
	InstanceID string `json:"instance_id"`
	Status     string `json:"status"`
}

type Project struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Description    *string `json:"description"`
	InstancesCount int64   `json:"instances_count"`
}

type Tag struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	InstancesCount int64  `json:"instances_count"`
}

type Organisation struct {
	InstanceID string   `json:"instance_id"`
	Project    *Project `json:"project"`
	Tags       []Tag    `json:"tags"`
}

type Operation struct {
	ID            FlexString      `json:"id"`
	Type          string          `json:"type"`
	Status        string          `json:"status"`
	Result        json.RawMessage `json:"result"`
	FailureReason *string         `json:"failure_reason"`
	InstanceID    string          `json:"instance_id"`
}

// CreateInstanceRequest is the body of POST /instances.
type CreateInstanceRequest struct {
	PlanID               string            `json:"plan_id"`
	Region               string            `json:"region,omitempty"`
	ImageID              string            `json:"image_id,omitempty"`
	MarketplaceVersionID string            `json:"marketplace_version_id,omitempty"`
	Name                 string            `json:"name,omitempty"`
	Password             string            `json:"password,omitempty"`
	SSHKeyIDs            []int64           `json:"ssh_key_ids,omitempty"`
	Billing              string            `json:"billing,omitempty"`
	AppFields            map[string]string `json:"app_fields,omitempty"`
}

// ---- catalogue ----

func (c *Client) ListRegions(ctx context.Context) ([]Region, error) {
	var out []Region
	return out, c.GetData(ctx, "/regions", nil, &out)
}

func (c *Client) ListPlans(ctx context.Context) ([]Plan, error) {
	var out []Plan
	return out, c.GetData(ctx, "/plans", nil, &out)
}

func (c *Client) ListImages(ctx context.Context) ([]Image, error) {
	var out []Image
	return out, c.GetData(ctx, "/images", nil, &out)
}

func (c *Client) ListApplications(ctx context.Context) ([]Application, error) {
	var out []Application
	return out, c.GetData(ctx, "/applications", nil, &out)
}

// ---- instances ----

func (c *Client) CreateInstance(ctx context.Context, req CreateInstanceRequest, idempotencyKey string) (*Operation, error) {
	resp, err := c.Do(ctx, http.MethodPost, "/instances", nil, req, map[string]string{"Idempotency-Key": idempotencyKey})
	if err != nil {
		return nil, err
	}
	var op Operation
	if err := decodeData(resp, &op); err != nil {
		return nil, err
	}
	return &op, nil
}

func (c *Client) GetInstance(ctx context.Context, id string) (*Instance, error) {
	var out Instance
	if err := c.GetData(ctx, "/instances/"+url.PathEscape(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ListInstances(ctx context.Context) ([]Instance, error) {
	return ListAll[Instance](ctx, c, "/instances", 100)
}

func (c *Client) GetInstanceStatus(ctx context.Context, id string) (*InstanceStatus, error) {
	var out InstanceStatus
	if err := c.GetData(ctx, "/instances/"+url.PathEscape(id)+"/status", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// InstanceAction runs start, stop, shutdown, reboot or hard-reboot.
func (c *Client) InstanceAction(ctx context.Context, id, action string) (*Operation, error) {
	resp, err := c.Do(ctx, http.MethodPost, "/instances/"+url.PathEscape(id)+"/"+action, nil, nil, nil)
	if err != nil {
		return nil, err
	}
	var op Operation
	return &op, decodeData(resp, &op)
}

func (c *Client) ResizeInstance(ctx context.Context, id, planID string) (*Operation, error) {
	resp, err := c.Do(ctx, http.MethodPost, "/instances/"+url.PathEscape(id)+"/resize", nil, map[string]any{"plan_id": planID}, nil)
	if err != nil {
		return nil, err
	}
	var op Operation
	return &op, decodeData(resp, &op)
}

// DeleteInstance returns the operation, or nil if the API answered without
// one. The API refuses with 409 deletion_protected while protection is on.
func (c *Client) DeleteInstance(ctx context.Context, id, confirmName string) (*Operation, error) {
	resp, err := c.Do(ctx, http.MethodDelete, "/instances/"+url.PathEscape(id), nil, map[string]any{"confirm": confirmName}, nil)
	if err != nil {
		return nil, err
	}
	if len(resp.Data) == 0 {
		return nil, nil
	}
	var op Operation
	return &op, decodeData(resp, &op)
}

func (c *Client) GetOrganisation(ctx context.Context, id string) (*Organisation, error) {
	var out Organisation
	if err := c.GetData(ctx, "/instances/"+url.PathEscape(id)+"/organisation", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SetProject assigns a project; projectID "" clears it.
func (c *Client) SetProject(ctx context.Context, id, projectID string) error {
	var pid any
	if projectID != "" {
		pid = projectID
	}
	_, err := c.Do(ctx, http.MethodPut, "/instances/"+url.PathEscape(id)+"/project", nil, map[string]any{"project_id": pid}, nil)
	return err
}

// SetTags replaces the instance's tags (mode sync).
func (c *Client) SetTags(ctx context.Context, id string, tagIDs []string) error {
	if tagIDs == nil {
		tagIDs = []string{}
	}
	_, err := c.Do(ctx, http.MethodPut, "/instances/"+url.PathEscape(id)+"/tags", nil, map[string]any{"tag_ids": tagIDs, "mode": "sync"}, nil)
	return err
}

// ---- operations ----

func (c *Client) GetOperation(ctx context.Context, id string) (*Operation, time.Duration, error) {
	resp, err := c.Do(ctx, http.MethodGet, "/operations/"+url.PathEscape(id), nil, nil, nil)
	if err != nil {
		return nil, 0, err
	}
	var op Operation
	if err := decodeData(resp, &op); err != nil {
		return nil, 0, err
	}
	return &op, resp.RetryAfter, nil
}

// WaitOperation polls until the operation succeeds or fails, or ctx ends.
func (c *Client) WaitOperation(ctx context.Context, op *Operation) (*Operation, error) {
	if op == nil || op.ID == "" {
		return op, nil
	}
	cur := op
	for {
		switch cur.Status {
		case "success":
			return cur, nil
		case "error", "failed":
			reason := "no reason given"
			if cur.FailureReason != nil && *cur.FailureReason != "" {
				reason = *cur.FailureReason
			}
			return cur, fmt.Errorf("operation %s (%s) failed: %s", cur.ID, cur.Type, reason)
		}
		next, wait, err := c.GetOperation(ctx, string(cur.ID))
		if err != nil {
			return cur, fmt.Errorf("polling operation %s: %w", cur.ID, err)
		}
		cur = next
		if cur.Status == "success" || cur.Status == "error" || cur.Status == "failed" {
			continue
		}
		if wait <= 0 {
			wait = c.PollInterval
		}
		if err := c.Sleep(ctx, wait); err != nil {
			return cur, fmt.Errorf("waiting for operation %s: %w", cur.ID, err)
		}
	}
}

// ---- projects & tags ----

func (c *Client) CreateProject(ctx context.Context, name string, description *string) (*Project, error) {
	resp, err := c.Do(ctx, http.MethodPost, "/projects", nil, map[string]any{"name": name, "description": description}, nil)
	if err != nil {
		return nil, err
	}
	var p Project
	return &p, decodeData(resp, &p)
}

func (c *Client) GetProject(ctx context.Context, id string) (*Project, error) {
	var p Project
	if err := c.GetData(ctx, "/projects/"+url.PathEscape(id), nil, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (c *Client) UpdateProject(ctx context.Context, id, name string, description *string) (*Project, error) {
	resp, err := c.Do(ctx, http.MethodPatch, "/projects/"+url.PathEscape(id), nil, map[string]any{"name": name, "description": description}, nil)
	if err != nil {
		return nil, err
	}
	var p Project
	return &p, decodeData(resp, &p)
}

func (c *Client) CreateTag(ctx context.Context, name string) (*Tag, error) {
	resp, err := c.Do(ctx, http.MethodPost, "/tags", nil, map[string]any{"name": name}, nil)
	if err != nil {
		return nil, err
	}
	var t Tag
	return &t, decodeData(resp, &t)
}

func (c *Client) GetTag(ctx context.Context, id string) (*Tag, error) {
	var t Tag
	if err := c.GetData(ctx, "/tags/"+url.PathEscape(id), nil, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (c *Client) UpdateTag(ctx context.Context, id, name string) (*Tag, error) {
	resp, err := c.Do(ctx, http.MethodPatch, "/tags/"+url.PathEscape(id), nil, map[string]any{"name": name}, nil)
	if err != nil {
		return nil, err
	}
	var t Tag
	return &t, decodeData(resp, &t)
}
