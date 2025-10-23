package syro

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strings"
	"time"
)

// NOTE: util method which can generate a curl request for easier sharing of requests
// NOTE: maybe rename this to Fetch or Curl ?
type Request struct {
	Method            string
	URL               string
	Headers           map[string]string
	Body              []byte
	ignoreStatusCodes bool
	client            *http.Client // Optional custom HTTP client, if nil, default client will be used
	errBodyLimit      *int         // Optional limit for error body size, if nil, no limit
}

type Response struct {
	Request    *Request
	Body       []byte
	Header     http.Header
	StatusCode int
	Duration   time.Duration
}

func NewRequest(method, url string) *Request {
	defaultErrBodyLimit := 1000

	return &Request{
		Method:       method,
		URL:          url,
		Headers:      make(map[string]string),
		client:       &http.Client{},
		errBodyLimit: &defaultErrBodyLimit,
	}
}

func (r *Request) WithHeaders(headers map[string]string) *Request {
	maps.Copy(r.Headers, headers)
	return r
}

func (r *Request) WithBasicAuth(username, password string) *Request {
	auth := fmt.Sprintf("%s:%s", username, password)
	encoded := base64.StdEncoding.EncodeToString([]byte(auth))
	r.Headers["Authorization"] = "Basic " + encoded
	return r
}

// WithErrorBodyLimit sets a limit for the error body size. If nil, no limit is applied.
func (r *Request) WithErrorBodyLimit(limit *int) *Request {
	r.errBodyLimit = limit
	return r
}

func (r *Request) WithHeader(key, value string) *Request {
	r.Headers[key] = value
	return r
}

func (r *Request) WithJsonHeader() *Request {
	r.Headers["Content-Type"] = "application/json"
	return r
}

func (r *Request) WithBody(body []byte) *Request {
	r.Body = body
	return r
}

func (r *Request) WithIgnoreStatusCodes(ignore bool) *Request {
	r.ignoreStatusCodes = ignore
	return r
}

func (r *Request) WithClient(c *http.Client) *Request {
	if c != nil {
		r.client = c
	}
	return r
}

func (r *Request) Do() (*Response, error) {

	now := time.Now()

	url := r.URL

	if r.Method == "" {
		return nil, fmt.Errorf("request method is not set")
	}

	if url == "" {
		return nil, fmt.Errorf("request URL is not set")
	}

	var reqBody []byte
	if r.Body != nil {
		reqBody = r.Body
	}

	req, err := http.NewRequest(r.Method, url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}

	for k, v := range r.Headers {
		req.Header.Set(k, v)
	}

	client := r.client

	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error fetching %v : %v", r.URL, err)
	}
	defer res.Body.Close()

	dur := time.Since(now)

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body when fetching %v, status %v: %v, error: ", url, res.Status, err)
	}

	responseData := &Response{
		StatusCode: res.StatusCode,
		Header:     res.Header,
		Body:       body,
		Duration:   dur,
		Request:    r,
	}

	if r.ignoreStatusCodes {
		return responseData, nil
	}

	if res.StatusCode != 200 && res.StatusCode != 201 && res.StatusCode != 202 {
		bodyStr := ""
		if body != nil && r.errBodyLimit != nil {
			bodyUpTo := min(len(body), *r.errBodyLimit) // limit to x characters
			bodyStr = string(body[:bodyUpTo])
		}

		return responseData, fmt.Errorf("response did not return status in 200 group while requesting %v, status: %v, body: %v", url, res.Status, bodyStr)
	}

	if body == nil {
		return responseData, fmt.Errorf("response returned empty body while requesting %v", url)
	}

	return responseData, nil
}

func (r *Request) AsCURL() string {
	var buf bytes.Buffer

	buf.WriteString("curl")

	hasOptions := false

	// Add method if not GET
	if r.Method != "" && r.Method != "GET" {
		buf.WriteString(fmt.Sprintf(" -X %s \\\n", r.Method))
		hasOptions = true
	}

	// Add headers
	for k, v := range r.Headers {
		buf.WriteString(fmt.Sprintf("  -H '%s: %s' \\\n", k, v))
		hasOptions = true
	}

	// Add body if present
	if len(r.Body) > 0 {
		// Escape single quotes in the body
		b := bytes.ReplaceAll(r.Body, []byte("'"), []byte("'\\''"))
		buf.WriteString(fmt.Sprintf("  -d '%s' \\\n", string(b)))
		hasOptions = true
	}

	// Add URL (always last, no trailing backslash)
	if hasOptions {
		buf.WriteString(fmt.Sprintf("  \"%s\"", r.URL))
	} else {
		buf.WriteString(fmt.Sprintf(" \"%s\"", r.URL))
	}

	return buf.String()
}

func (r *Response) Prettified() string {
	buf := bytes.Buffer{}

	buf.WriteString(fmt.Sprintf("Status: %d\n", r.StatusCode))
	buf.WriteString(fmt.Sprintf("Duration: %v\n", r.Duration))

	buf.WriteString("Headers:\n")
	for k, v := range r.Header {
		buf.WriteString(fmt.Sprintf("  %s: %s\n", k, v))
	}
	buf.WriteString("\n")

	if r.Body != nil {
		buf.WriteString("Body:\n\n")
		buf.Write(r.Body)
		buf.WriteString("\n")
	}

	return buf.String()
}

func (r *Response) Info() string {

	formatBody := func(data []byte) string {
		var obj any
		if err := json.Unmarshal(data, &obj); err != nil {
			return string(data)
		}
		pretty, err := json.MarshalIndent(obj, "", "  ")
		if err != nil {
			return string(data)
		}
		return string(pretty)
	}

	var sb strings.Builder

	// Request section
	sb.WriteString("┌─ REQUEST ─────────────────────────────────────────────\n")
	sb.WriteString(r.Request.AsCURL())
	sb.WriteString("\n")
	sb.WriteString("└───────────────────────────────────────────────────────\n\n")

	// Response section
	sb.WriteString("┌─ RESPONSE ────────────────────────────────────────────\n")
	sb.WriteString(fmt.Sprintf("│ Status: %d\n", r.StatusCode))
	sb.WriteString(fmt.Sprintf("│ Duration: %v\n", r.Duration))

	// Headers
	if len(r.Header) > 0 {
		sb.WriteString("│\n│ Headers:\n")
		for key, values := range r.Header {
			sb.WriteString(fmt.Sprintf("│   %s: %s\n", key, strings.Join(values, ", ")))
		}
	}

	// Body
	sb.WriteString("│\n│ Body:\n")
	bodyStr := formatBody(r.Body)
	for _, line := range strings.Split(bodyStr, "\n") {
		if line != "" {
			sb.WriteString("│   " + line + "\n")
		}
	}
	sb.WriteString("└───────────────────────────────────────────────────────\n")

	return sb.String()
}
