package enrich

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type Client struct {
	apiKey   string
	include  string
	fields   string
	excludes string
	lang     string
	http     *http.Client
}

func NewIPGeolocationClient(apiKey, include, fields, excludes, lang string) *Client {
	return &Client{
		apiKey:   apiKey,
		include:  include,
		fields:   fields,
		excludes: excludes,
		lang:     lang,
		http: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

func (c *Client) Lookup(ctx context.Context, ip string) (map[string]any, error) {
	u, _ := url.Parse("https://api.ipgeolocation.io/v2/ipgeo")
	q := u.Query()

	q.Set("apiKey", c.apiKey)
	q.Set("ip", ip)

	if c.include != "" {
		q.Set("include", c.include)
	}
	if c.fields != "" {
		q.Set("fields", c.fields)
	}
	if c.excludes != "" {
		q.Set("excludes", c.excludes)
	}
	if c.lang != "" {
		q.Set("lang", c.lang)
	}

	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ipgeolocation api error: %d", resp.StatusCode)
	}

	var data map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return data, nil
}
