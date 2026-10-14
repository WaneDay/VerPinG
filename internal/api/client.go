package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Client talks to the mihomo / Clash Verge Rev external controller.
type Client struct {
	base string
	sec  string
	hc   *http.Client
}

// New builds a client for base URL and optional secret.
func New(base, sec string, timeout time.Duration) *Client {
	return &Client{base: base, sec: sec, hc: &http.Client{Timeout: timeout}}
}

func (c *Client) do(req *http.Request) (*http.Response, error) {
	if c.sec != "" {
		req.Header.Set("Authorization", "Bearer "+c.sec)
	}
	req.Header.Set("User-Agent", "VerPinG/1.0")
	return c.hc.Do(req)
}

// ProxiesResp is the payload of GET /proxies.
type ProxiesResp struct {
	Proxies map[string]Proxy `json:"proxies"`
}

// Proxy is a single proxy / group entry.
type Proxy struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Now     string   `json:"now"`
	All     []string `json:"all"`
	History []Hist   `json:"history"`
}

// Hist is one delay-history sample.
type Hist struct {
	Time  string `json:"time"`
	Delay int    `json:"delay"`
}

// GetProxies fetches all proxies and groups.
func (c *Client) GetProxies() (*ProxiesResp, error) {
	req, _ := http.NewRequest("GET", c.base+"/proxies", nil)
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var pr ProxiesResp
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

// PutSelect selects node inside group.
func (c *Client) PutSelect(group, node string) error {
	body, _ := json.Marshal(map[string]string{"name": node})
	req, _ := http.NewRequest("PUT", c.base+"/proxies/"+url.PathEscape(group), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("put select %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// PatchModeOff sets the core to direct mode (closes the system proxy path).
func (c *Client) PatchModeOff() error {
	body, _ := json.Marshal(map[string]string{"mode": "off"})
	req, _ := http.NewRequest("PATCH", c.base+"/configs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("patch mode off %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// GetVersion checks whether the controller is reachable.
func (c *Client) GetVersion() error {
	req, _ := http.NewRequest("GET", c.base+"/version", nil)
	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// DelayTest actively tests a single node delay via the controller.
func (c *Client) DelayTest(node, testURL string, delayTimeoutMs int) int {
	q := url.Values{}
	q.Set("url", testURL)
	q.Set("timeout", strconv.Itoa(delayTimeoutMs))
	req, _ := http.NewRequest("GET", c.base+"/proxies/"+url.PathEscape(node)+"/delay?"+q.Encode(), nil)
	resp, err := c.do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	var body struct {
		Delay int `json:"delay"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return 0
	}
	if body.Delay <= 0 {
		return 0
	}
	return body.Delay
}
