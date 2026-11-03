package helper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"
)

type Client struct {
	httpClient *http.Client
}

func NewClient(socketPath string) *Client {
	transport := &http.Transport{
		DialContext: func(
			ctx context.Context,
			network string,
			addr string,
		) (net.Conn, error) {
			return (&net.Dialer{
				Timeout: 5 * time.Second,
			}).DialContext(ctx, "unix", socketPath)
		},
	}

	return &Client{
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   35 * time.Second,
		},
	}
}

func (c *Client) Execute(
	ctx context.Context,
	action string,
) (*Response, error) {

	allowed := map[string]bool{
		"apache.status":     true,
		"apache.configtest": true,
		"apache.reload":     true,
		"apache.restart":    true,
	}

	if !allowed[action] {
		return nil, fmt.Errorf("operation not allowed")
	}

	payload, err := json.Marshal(Request{
		Action: action,
	})

	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		"http://localhost/v1/execute",
		bytes.NewReader(payload),
	)

	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result Response

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK || !result.Success {
		return &result, fmt.Errorf("helper operation failed: %s", result.Error)
	}

	return &result, nil
}
