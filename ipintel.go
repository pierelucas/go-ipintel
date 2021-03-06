// Package ipintel is a simple Go wrapper for the getipintel.net
// proxy detection API
package ipintel

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/juju/ratelimit"
)

const (
	version = "0.2.0"
	urlBase = "check.getipintel.net/check.php"
)

var (
	// throttle queries to ~15 req/min with a burst capacity of 15 (imposed by API)
	rateLimiter = ratelimit.NewBucketWithQuantum(4*time.Second, 15, 1)

	httpClient = http.Client{Timeout: 10 * time.Second}
	userAgent  = "go-ipintel/" + version + " (github.com/janeczku/go-ipintel)"
)

// CheckType represents the type of check used to determine the proxy score.
type CheckType string

const (
	// Static uses static lists to determine if the IP is a proxy.
	// The returned score is either 0 or 1.
	Static CheckType = "m"
	// Dynamic uses static lists and machine learning to determine if the IP is
	// a proxy. The returned score is a floating number between 0 and 1.
	Dynamic CheckType = "b"
)

// Client is a struct used to make API queries.
type Client struct {
	// Your email address
	Email string
	// Scheme used for the API requests ("http" or "https")
	Scheme string
	// Type of proxy check to use (Static/Dynamic)
	Check CheckType
	// Maximum time to wait when a query is being throttled.
	// If set to zero calls to GetProxyScore() will block until
	// there is enough capacity in the rate limiter bucket.
	MaxWait time.Duration
}

// NewClient creates a new Client using the given parameters.
// Example:
//   c := ipintel.NewClient("your@email.com", false, ipintel.Static, 5*time.Second)
func NewClient(email string, ssl bool, check CheckType, mWait time.Duration) *Client {
	scheme := "http"
	if ssl {
		scheme = "https"
	}
	return &Client{
		Email:   email,
		Scheme:  scheme,
		Check:   check,
		MaxWait: mWait,
	}
}

// GetProxyScore queries the API and returns the proxy score for the given IP address.
func (c *Client) GetProxyScore(ip string) (score float32, err error) {
	if ok := rateLimiter.WaitMaxDuration(1, c.MaxWait); !ok {
		err = fmt.Errorf("Throttled: Can't make query within the next %s", c.MaxWait)
		return
	}

	req, err := http.NewRequest("GET", c.getURL(ip), nil)
	if err != nil {
		err = fmt.Errorf("Failed preparing request: %v", err)
		return
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		err = fmt.Errorf("Failed to query API: %v", err)
		return
	}

	if resp.StatusCode == 429 {
		err = fmt.Errorf("API error: Rate limit exceeded")
		return
	}

	type response struct {
		Status string  `json:"status"`
		ErrMsg string  `json:"message"`
		Score  float32 `json:"result,string"`
	}

	decoder := json.NewDecoder(resp.Body)
	defer resp.Body.Close()

	var respObj response
	if err = decoder.Decode(&respObj); err != nil {
		err = fmt.Errorf("Failed to parse API response: %v", err)
		return
	}

	if respObj.Status != "success" {
		err = fmt.Errorf("API error: %s", respObj.ErrMsg)
		return
	}

	return respObj.Score, nil
}

func (c *Client) getURL(ip string) string {
	return fmt.Sprintf("%s://%s?ip=%s&contact=%s&flags=%s&format=json",
		c.Scheme, urlBase, ip, c.Email, c.Check)
}
