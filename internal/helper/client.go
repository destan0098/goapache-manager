package helper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	return c.executeRequest(ctx, Request{
		Action: action,
	})
}

func (c *Client) CreateVHost(
	ctx context.Context,
	domain string,
) (*Response, error) {
	return c.executeRequest(ctx, Request{
		Action: "apache.vhost.create",
		Domain: domain,
	})
}
func (c *Client) EnableVHost(
	ctx context.Context,
	domain string,
) (*Response, error) {
	return c.executeRequest(ctx, Request{
		Action: "apache.vhost.enable",
		Domain: domain,
	})
}

func (c *Client) DisableVHost(
	ctx context.Context,
	domain string,
) (*Response, error) {
	return c.executeRequest(ctx, Request{
		Action: "apache.vhost.disable",
		Domain: domain,
	})
}

func (c *Client) EditVHost(
	ctx context.Context,
	domain string,
	documentRoot string,
	serverAliases []string,
) (*Response, error) {
	return c.executeRequest(ctx, Request{
		Action:        "apache.vhost.edit",
		Domain:        domain,
		DocumentRoot:  documentRoot,
		ServerAliases: serverAliases,
	})
}

func (c *Client) executeRequest(
	ctx context.Context,
	request Request,
) (*Response, error) {

	allowed := map[string]bool{
		"apache.status":        true,
		"apache.configtest":    true,
		"apache.reload":        true,
		"apache.restart":       true,
		"apache.vhosts.list":   true,
		"apache.vhost.create":  true,
		"apache.vhost.enable":  true,
		"apache.vhost.disable": true,
		"apache.vhost.edit":    true,
	}

	if !allowed[request.Action] {
		return nil, fmt.Errorf("operation not allowed")
	}

	payload, err := json.Marshal(request)
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

	decoder := json.NewDecoder(
		io.LimitReader(resp.Body, 64*1024),
	)

	if err := decoder.Decode(&result); err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK || !result.Success {
		return &result, fmt.Errorf(
			"helper operation failed: %s",
			result.Error,
		)
	}

	return &result, nil
}
