// api.go — minimal REST client for the kumomta admin-service.
//
// Hand-rolled rather than reusing any generated client: the loadgen runs in
// its own process with no shared imports beyond the stdlib + yaml, and we
// only need a handful of endpoints (login + create resources). Idempotent
// by design — every helper handles "already exists" as success.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// AdminClient holds the bearer token + base URL. Reused across all setup
// calls so the token is fetched once.
type AdminClient struct {
	BaseURL string // e.g. "http://admin-service:8000"
	Token   string
	HTTP    *http.Client
}

func NewAdminClient(baseURL string) *AdminClient {
	return &AdminClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTP:    &http.Client{Timeout: 15 * time.Second},
	}
}

// Login obtains an access token. Caller is responsible for handling boot
// races (admin-service may not be up yet) — see WaitForAdmin.
func (c *AdminClient) Login(user, pass string) error {
	body, _ := json.Marshal(map[string]string{"username": user, "password": pass})
	req, _ := http.NewRequest("POST", c.BaseURL+"/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("login: status %d: %s", resp.StatusCode, b)
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("login decode: %w", err)
	}
	c.Token = out.AccessToken
	return nil
}

// WaitForAdmin polls /v1/auth/whoami until it succeeds or the timeout hits.
// Used during boot to wait out the admin-service's first-write migrations.
func (c *AdminClient) WaitForAdmin(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := c.HTTP.Get(c.BaseURL + "/v1/auth/whoami")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode < 500 {
				return nil
			}
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("admin-service did not come up within %s", timeout)
}

// post is a thin helper that auths the request, JSON-encodes the body, and
// returns the parsed response. notFoundOK silences 4xx errors that are
// expected for "already exists" idempotent retries.
func (c *AdminClient) post(path string, body any, out any) error {
	return c.req("POST", path, body, out)
}
func (c *AdminClient) put(path string, body any, out any) error {
	return c.req("PUT", path, body, out)
}
func (c *AdminClient) req(method, path string, body any, out any) error {
	var buf io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		buf = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.BaseURL+path, buf)
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		// "duplicate" / unique-constraint errors are expected on idempotent
		// re-runs. Bubble them up as a typed error the caller can ignore.
		if strings.Contains(string(b), "duplicate") || strings.Contains(string(b), "already exists") {
			return errAlreadyExists{}
		}
		return fmt.Errorf("%s %s: status %d: %s", method, path, resp.StatusCode, b)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

type errAlreadyExists struct{}

func (errAlreadyExists) Error() string { return "already exists" }

func isAlreadyExists(err error) bool {
	_, ok := err.(errAlreadyExists)
	return ok
}

// --- typed wrappers ---------------------------------------------------------

func (c *AdminClient) CreateVMTA(v SetupVMTA) (int, error) {
	body := map[string]any{
		"name":            v.Name,
		"helo_name":       v.HeloName,
		"source_ips":      v.SourceIPs,
		"max_connections": v.MaxConn,
	}
	var out struct {
		ID int `json:"id"`
	}
	if err := c.post("/v1/vmtas", body, &out); err != nil {
		if !isAlreadyExists(err) {
			return 0, err
		}
		// Already in the DB — use the existing row. VMTA has no PUT
		// endpoint and its FK from virtual_mta_group_members blocks
		// delete-and-recreate. To intentionally apply a different config,
		// docker compose down -v before re-running.
		return c.findVMTAID(v.Name)
	}
	return out.ID, nil
}

// del is a thin wrapper that ignores 404 (the resource was already gone).
func (c *AdminClient) del(path string) error {
	req, _ := http.NewRequest("DELETE", c.BaseURL+path, nil)
	req.Header.Set("Authorization", "Bearer "+c.Token)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("DELETE %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 && resp.StatusCode != 404 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("DELETE %s: status %d: %s", path, resp.StatusCode, b)
	}
	return nil
}

func (c *AdminClient) findVMTAID(name string) (int, error) {
	req, _ := http.NewRequest("GET", c.BaseURL+"/v1/vmtas", nil)
	req.Header.Set("Authorization", "Bearer "+c.Token)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	var page struct {
		Items []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"items"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&page)
	for _, it := range page.Items {
		if it.Name == name {
			return it.ID, nil
		}
	}
	return 0, fmt.Errorf("vmta %q not found", name)
}

func (c *AdminClient) CreateGroup(g SetupGroup, vmtaIDs map[string]int) error {
	body := map[string]any{"name": g.Name, "enabled": true}
	var out struct {
		ID int `json:"id"`
	}
	gid := 0
	if err := c.post("/v1/vmta-groups", body, &out); err != nil {
		if !isAlreadyExists(err) {
			return err
		}
		// fetch existing
		req, _ := http.NewRequest("GET", c.BaseURL+"/v1/vmta-groups", nil)
		req.Header.Set("Authorization", "Bearer "+c.Token)
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		var page struct {
			Items []struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
			} `json:"items"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&page)
		for _, it := range page.Items {
			if it.Name == g.Name {
				gid = it.ID
				break
			}
		}
		if gid == 0 {
			return fmt.Errorf("group %q not found after duplicate", g.Name)
		}
	} else {
		gid = out.ID
	}
	members := make([]map[string]any, 0, len(g.Members))
	for _, m := range g.Members {
		vid, ok := vmtaIDs[m.Vmta]
		if !ok {
			return fmt.Errorf("group %q references unknown vmta %q", g.Name, m.Vmta)
		}
		members = append(members, map[string]any{
			"vmta_id":  vid,
			"weight":   m.Weight,
			"priority": 0,
			"enabled":  true,
		})
	}
	return c.put(fmt.Sprintf("/v1/vmta-groups/%d/members", gid), map[string]any{"members": members}, nil)
}

func (c *AdminClient) CreateMailClass(mc SetupMailClass) error {
	body := map[string]any{
		"name":        mc.Name,
		"enabled":     true,
		"target_kind": mc.TargetKind,
		"target_ref":  mc.TargetRef,
	}
	if err := c.post("/v1/mail-classes", body, nil); err != nil {
		if !isAlreadyExists(err) {
			return err
		}
	}
	return nil
}

func (c *AdminClient) CreateRule(r SetupRule) error {
	// Convert simple `when: { to_domain: foo }` to the routing condition shape.
	conditions := []map[string]any{}
	for k, v := range r.When {
		conditions = append(conditions, map[string]any{
			"field": k,
			"op":    "equals",
			"value": fmt.Sprintf("%v", v),
		})
	}
	body := map[string]any{
		"name":       r.Name,
		"priority":   r.Priority,
		"enabled":    true,
		"conditions": conditions,
		"target": map[string]any{
			"kind": r.TargetK,
			"ref":  r.TargetRef,
		},
	}
	if err := c.post("/v1/routing", body, nil); err != nil {
		if !isAlreadyExists(err) {
			return err
		}
	}
	return nil
}

func (c *AdminClient) CreateSuppression(s SetupSuppress) error {
	body := map[string]any{
		"address": s.Address,
		"scope":   s.Scope,
		"reason":  s.Reason,
	}
	if err := c.post("/v1/suppressions", body, nil); err != nil {
		if !isAlreadyExists(err) {
			return err
		}
	}
	return nil
}

func (c *AdminClient) ApplyPolicy() error {
	// Reload step is best-effort: the admin-service writes init.lua even if
	// the kumomta reload endpoint 404s, and the harness restarts kumomta
	// via compose anyway.
	_ = c.post("/v1/policy/apply", map[string]any{}, nil)
	return nil
}

// Teardown removes every entity the scenario seeded — and the auto-created
// suppressions FBL processing produces — leaving the admin-service DB in a
// production-clean state. Run after asserts so an interactive operator can
// still inspect the same shared deployment without test artifacts polluting
// the rendered policy.
//
// Order matters: classes/rules reference vmtas+groups; groups reference
// vmtas. Delete in reverse-dependency order. Suppressions are independent.
//
// `extraSuppressionAddrs` is the set of recipient addresses that traffic ran
// against — auto-suppress fires on FBL receipts so e.g. "a@fbl.test" gets a
// row even though the scenario didn't declare it. We sweep those too.
func (c *AdminClient) Teardown(s SetupBlock, extraSuppressionAddrs []string) error {
	// Build lookup tables by name → id for each entity type, with one GET each.
	mc, err := c.listByName("/v1/mail-classes")
	if err != nil {
		return fmt.Errorf("list mail-classes: %w", err)
	}
	for _, x := range s.Classes {
		if id, ok := mc[x.Name]; ok {
			if err := c.del(fmt.Sprintf("/v1/mail-classes/%d", id)); err != nil {
				return fmt.Errorf("delete mail-class %s: %w", x.Name, err)
			}
		}
	}

	rules, err := c.listByName("/v1/routing")
	if err != nil {
		return fmt.Errorf("list routing: %w", err)
	}
	for _, x := range s.Rules {
		if id, ok := rules[x.Name]; ok {
			if err := c.del(fmt.Sprintf("/v1/routing/%d", id)); err != nil {
				return fmt.Errorf("delete rule %s: %w", x.Name, err)
			}
		}
	}

	groups, err := c.listByName("/v1/vmta-groups")
	if err != nil {
		return fmt.Errorf("list vmta-groups: %w", err)
	}
	for _, x := range s.Groups {
		if id, ok := groups[x.Name]; ok {
			if err := c.del(fmt.Sprintf("/v1/vmta-groups/%d", id)); err != nil {
				return fmt.Errorf("delete group %s: %w", x.Name, err)
			}
		}
	}

	vmtas, err := c.listByName("/v1/vmtas")
	if err != nil {
		return fmt.Errorf("list vmtas: %w", err)
	}
	for _, x := range s.VMTAs {
		if id, ok := vmtas[x.Name]; ok {
			if err := c.del(fmt.Sprintf("/v1/vmtas/%d", id)); err != nil {
				return fmt.Errorf("delete vmta %s: %w", x.Name, err)
			}
		}
	}

	// Suppressions index by address rather than name, and the IDs are UUID
	// strings — so use a dedicated path.
	addrs := map[string]bool{}
	for _, x := range s.Suppressions {
		addrs[x.Address] = true
	}
	for _, a := range extraSuppressionAddrs {
		addrs[a] = true
	}
	if len(addrs) > 0 {
		supp, err := c.listSuppressionsByAddress()
		if err != nil {
			return fmt.Errorf("list suppressions: %w", err)
		}
		for a := range addrs {
			if id, ok := supp[a]; ok {
				if err := c.del("/v1/suppressions/" + id); err != nil {
					return fmt.Errorf("delete suppression %s: %w", a, err)
				}
			}
		}
	}
	return nil
}

// listByName GETs a paginated list endpoint and returns name → numeric id.
// Items without an `id` or `name` are skipped silently — matches the
// resilience of the rest of the harness.
func (c *AdminClient) listByName(path string) (map[string]int, error) {
	req, _ := http.NewRequest("GET", c.BaseURL+path+"?limit=10000", nil)
	req.Header.Set("Authorization", "Bearer "+c.Token)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var page struct {
		Items []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"items"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&page)
	out := map[string]int{}
	for _, it := range page.Items {
		if it.Name != "" && it.ID != 0 {
			out[it.Name] = it.ID
		}
	}
	return out, nil
}

func (c *AdminClient) listSuppressionsByAddress() (map[string]string, error) {
	req, _ := http.NewRequest("GET", c.BaseURL+"/v1/suppressions?limit=10000", nil)
	req.Header.Set("Authorization", "Bearer "+c.Token)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var page struct {
		Items []struct {
			ID      string `json:"id"`
			Address string `json:"address"`
		} `json:"items"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&page)
	out := map[string]string{}
	for _, it := range page.Items {
		if it.Address != "" && it.ID != "" {
			out[it.Address] = it.ID
		}
	}
	return out, nil
}

// CountByEventType GETs /v1/logs and counts by event_type. The `since`
// argument is intentionally subtracted by 5s before filtering — the API
// serialises `at` with second-resolution but the caller's `since` is
// sub-second, so events emitted in the same wall-clock second the test
// started would otherwise be dropped. The harness wipes the DB volume
// between runs so over-counting from prior runs isn't a concern.
func (c *AdminClient) CountByEventType(since time.Time) (map[string]int, int, error) {
	url := fmt.Sprintf("%s/v1/logs?limit=10000", c.BaseURL)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+c.Token)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	var page struct {
		Items []struct {
			EventType string    `json:"event_type"`
			At        time.Time `json:"at"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, 0, err
	}
	cutoff := since.Add(-5 * time.Second)
	counts := map[string]int{}
	for _, it := range page.Items {
		if !cutoff.IsZero() && it.At.Before(cutoff) {
			continue
		}
		counts[it.EventType]++
	}
	return counts, page.Total, nil
}

func (c *AdminClient) FeedbackTotal() (int, error) {
	url := c.BaseURL + "/v1/feedback?limit=1"
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+c.Token)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	var page struct {
		Total int `json:"total"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&page)
	return page.Total, nil
}

func (c *AdminClient) SuppressionsByReason() (map[string]int, error) {
	url := c.BaseURL + "/v1/suppressions?limit=10000"
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+c.Token)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var page struct {
		Items []struct {
			Reason string `json:"reason"`
		} `json:"items"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&page)
	out := map[string]int{}
	for _, it := range page.Items {
		out[it.Reason]++
	}
	return out, nil
}
