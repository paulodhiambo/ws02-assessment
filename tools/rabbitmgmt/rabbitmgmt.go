// Package rabbitmgmt publishes, reads and purges RabbitMQ messages through the
// management HTTP API, so scripts and tests need no AMQP client.
package rabbitmgmt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Client talks to one broker's management API on the default vhost.
type Client struct {
	BaseURL, User, Password string
	HTTP                    *http.Client
}

// New returns a client with a sensible timeout.
func New(baseURL, user, password string) *Client {
	return &Client{BaseURL: baseURL, User: user, Password: password, HTTP: &http.Client{Timeout: 15 * time.Second}}
}

func (c *Client) do(method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.BaseURL+path, reader)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.User, c.Password)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s -> %d: %s", method, path, resp.StatusCode, raw)
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}

// Publish sends payload to exchange with routingKey. contentType may be
// empty (the consumer then applies its own default).
func (c *Client) Publish(exchange, routingKey, payload, contentType string, headers map[string]string) error {
	props := map[string]any{"headers": headers, "delivery_mode": 2}
	if contentType != "" {
		props["content_type"] = contentType
	}
	var res struct {
		Routed bool `json:"routed"`
	}
	err := c.do("POST", "/api/exchanges/%2F/"+url.PathEscape(exchange)+"/publish", map[string]any{
		"properties": props, "routing_key": routingKey, "payload": payload, "payload_encoding": "string",
	}, &res)
	if err == nil && !res.Routed {
		err = fmt.Errorf("message to %s/%s was not routed to any queue", exchange, routingKey)
	}
	return err
}

// Message is one message read from a queue.
type Message struct {
	Payload    string         `json:"payload"`
	Headers    map[string]any `json:"headers"`
	Redelivery bool           `json:"redelivered"`
}

// Get removes and returns up to count messages from queue.
func (c *Client) Get(queue string, count int) ([]Message, error) {
	var raw []struct {
		Payload     string `json:"payload"`
		Redelivered bool   `json:"redelivered"`
		Properties  struct {
			Headers map[string]any `json:"headers"`
		} `json:"properties"`
	}
	if err := c.do("POST", "/api/queues/%2F/"+url.PathEscape(queue)+"/get", map[string]any{
		"count": count, "ackmode": "ack_requeue_false", "encoding": "auto",
	}, &raw); err != nil {
		return nil, err
	}
	out := make([]Message, 0, len(raw))
	for _, m := range raw {
		out = append(out, Message{Payload: m.Payload, Headers: m.Properties.Headers, Redelivery: m.Redelivered})
	}
	return out, nil
}

// Purge deletes every message in queue.
func (c *Client) Purge(queue string) error {
	return c.do("DELETE", "/api/queues/%2F/"+url.PathEscape(queue)+"/contents", nil, nil)
}
