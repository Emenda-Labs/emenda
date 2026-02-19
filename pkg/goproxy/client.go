package goproxy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/mod/module"
)

const (
	defaultProxy        = "https://proxy.golang.org,direct"
	httpClientTimeout   = 30 * time.Second
	defaultUserAgent    = "emenda/0.1.0"
	statusNotFound      = http.StatusNotFound
	statusGone          = http.StatusGone
)

// Client downloads module zip files from the Go module proxy.
type Client struct {
	httpClient *http.Client
	userAgent  string
	proxies    []string
}

// NewClient creates a Client that reads the GOPROXY environment variable to
// determine the proxy chain. If GOPROXY is unset, it defaults to
// "https://proxy.golang.org,direct".
func NewClient() *Client {
	goproxy := os.Getenv("GOPROXY")
	if strings.TrimSpace(goproxy) == "" {
		goproxy = defaultProxy
	}

	// The GOPROXY value is a comma- or pipe-separated list of proxy URLs.
	replacer := strings.NewReplacer("|", ",")
	normalized := replacer.Replace(goproxy)

	parts := strings.Split(normalized, ",")
	proxies := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			proxies = append(proxies, trimmed)
		}
	}

	return &Client{
		httpClient: &http.Client{Timeout: httpClientTimeout},
		userAgent:  defaultUserAgent,
		proxies:    proxies,
	}
}

// DownloadZip fetches the zip archive for the given module and version from the
// proxy chain. It returns the raw zip bytes on success.
func (c *Client) DownloadZip(ctx context.Context, mod, version string) ([]byte, error) {
	escapedMod, err := module.EscapePath(mod)
	if err != nil {
		return nil, fmt.Errorf("escaping module path %q: %w", mod, err)
	}

	for i, proxy := range c.proxies {
		switch proxy {
		case "direct":
			fmt.Fprintf(os.Stderr, "goproxy: direct mode not supported yet, skipping\n")
			continue
		case "off":
			fmt.Fprintf(os.Stderr, "goproxy: proxy chain contains 'off', stopping\n")
			return nil, fmt.Errorf("module %s@%s not found on any proxy", mod, version)
		}

		zipURL := fmt.Sprintf("%s/%s/@v/%s.zip", proxy, escapedMod, version)

		data, tryNext, fetchErr := c.fetch(ctx, zipURL)
		if fetchErr == nil {
			return data, nil
		}

		if tryNext && i < len(c.proxies)-1 {
			continue
		}

		return nil, fetchErr
	}

	return nil, fmt.Errorf("module %s@%s not found on any proxy", mod, version)
}

// fetch performs a single HTTP GET for the given URL.
// It returns (data, tryNext, error).
// tryNext signals that the caller should attempt the next proxy in the chain.
func (c *Client) fetch(ctx context.Context, url string) ([]byte, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, fmt.Errorf("building request for %s: %w", url, err)
	}
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Network-level error; let the caller decide whether to try the next proxy.
		return nil, true, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == statusNotFound || resp.StatusCode == statusGone {
		return nil, true, fmt.Errorf("proxy returned %d for %s", resp.StatusCode, url)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("unexpected status %d from %s", resp.StatusCode, url)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, fmt.Errorf("reading response body from %s: %w", url, err)
	}

	return data, false, nil
}
