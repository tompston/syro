package syro

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type HttpClient struct {
	client *http.Client
}

// Make http requests with the client reused
func NewHttpClient(c *http.Client) *HttpClient {
	if c == nil {
		c = http.DefaultClient
	}
	return &HttpClient{c}
}

func (c *HttpClient) Request(method, url string) *Request {
	return &Request{
		Method:  method,
		URL:     url,
		Headers: map[string]string{},
		client:  c.client,
		ctx:     ctx,
	}
}

type Request struct {
	ctx               context.Context
	Headers           map[string]string
	client            *http.Client // Optional custom HTTP client. If nil, default client will be used
	errBodyLimit      *int         // Optional limit for the returned body size on error. If nil, no limit
	Method            string
	URL               string
	Body              []byte
	ignoreStatusCodes bool
}

type Response struct {
	Request    *Request
	Header     http.Header
	Body       []byte
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
		ctx:          ctx,
	}
}

func (r *Request) WithHeaders(headers map[string]string) *Request {
	maps.Copy(r.Headers, headers)
	return r
}

func (r *Request) WithCtx(ctx context.Context) *Request {
	if ctx != nil {
		r.ctx = ctx
	}
	return r
}

func (r *Request) WithBearerToken(token string) *Request {
	r.Headers["Authorization"] = "Bearer " + token
	return r
}

func (r *Request) WithFormData(data map[string]string) *Request {
	form := url.Values{}
	for k, v := range data {
		form.Add(k, v)
	}
	r.Body = []byte(form.Encode())
	r.Headers["Content-Type"] = "application/x-www-form-urlencoded"
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
	r.client = c
	return r
}

func (r *Request) Do() (*Response, error) {

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

	req, err := http.NewRequestWithContext(r.ctx, r.Method, url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}

	for k, v := range r.Headers {
		req.Header.Set(k, v)
	}

	now := time.Now()

	res, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error fetching %v : %v", url, err)
	}
	defer res.Body.Close()

	dur := time.Since(now)

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body when fetching %v, status %v: error: %v", url, res.Status, err)
	}

	_response := &Response{
		StatusCode: res.StatusCode,
		Header:     res.Header,
		Body:       body,
		Duration:   dur,
		Request:    r,
	}

	if r.ignoreStatusCodes {
		return _response, nil
	}

	if res.StatusCode != 200 && res.StatusCode != 201 && res.StatusCode != 202 {
		bodyStr := ""
		if body != nil && r.errBodyLimit != nil {
			bodyUpTo := min(len(body), *r.errBodyLimit) // limit to x characters
			bodyStr = string(body[:bodyUpTo])
		}

		return _response, fmt.Errorf("response did not return status in 200 group while requesting %v, status: %v, body: %v", url, res.Status, bodyStr)
	}

	if body == nil {
		return _response, fmt.Errorf("response returned empty body while requesting %v", url)
	}

	return _response, nil
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

func (r *Response) Summary() string {

	var sb strings.Builder

	// Request section
	sb.WriteString("───── REQUEST ─────────────────────────\n")
	sb.WriteString(r.Request.AsCURL())
	sb.WriteString("\n\n")

	// Response section
	sb.WriteString("───── RESPONSE ────────────────────────\n")
	sb.WriteString(fmt.Sprintf("Status: %d\n", r.StatusCode))
	sb.WriteString(fmt.Sprintf("Duration: %v\n", r.Duration))

	// Headers
	if len(r.Header) > 0 {
		sb.WriteString("\nHeaders:\n")
		for key, values := range r.Header {
			sb.WriteString(fmt.Sprintf("  %s: %s\n", key, strings.Join(values, ", ")))
		}
	}

	// Body
	sb.WriteString("\nBody:\n")
	bodyStr := r.formatBody()
	for _, line := range strings.Split(bodyStr, "\n") {
		if line != "" {
			sb.WriteString("   " + line + "\n")
		}
	}

	return sb.String()
}

func (r *Response) formatBody() string {
	if r == nil {
		return ""
	}

	data := r.Body

	if len(data) == 0 {
		return "(empty)"
	}

	// Trim whitespace to check actual content
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return "(empty)"
	}

	// Check Content-Type header first
	contentType := r.Header.Get("Content-Type")

	// Try JSON - check header or content structure
	if strings.Contains(contentType, "application/json") ||
		strings.Contains(contentType, "text/json") ||
		bytes.HasPrefix(trimmed, []byte("{")) ||
		bytes.HasPrefix(trimmed, []byte("[")) {
		var obj any
		if err := json.Unmarshal(trimmed, &obj); err == nil {
			pretty, err := json.MarshalIndent(obj, "", "  ")
			if err == nil {
				return string(pretty)
			}
		}
	}

	// Try XML - check header or content structure
	if strings.Contains(contentType, "application/xml") ||
		strings.Contains(contentType, "text/xml") ||
		(bytes.HasPrefix(trimmed, []byte("<")) && bytes.HasSuffix(trimmed, []byte(">"))) {

		// Additional check: verify it's not HTML
		lowerContent := strings.ToLower(string(trimmed[:min(len(trimmed), 200)]))
		isHTML := strings.Contains(lowerContent, "<!doctype html") || strings.Contains(lowerContent, "<html")

		if !isHTML {
			content, err := prettyPrintXML(string(data))
			if err == nil {
				return content
			}
		}
	}

	// Fall back to plain string
	return string(data)
}

func prettyPrintXML(xmlString string) (string, error) {
	// Create a decoder
	decoder := xml.NewDecoder(strings.NewReader(xmlString))

	// Read tokens and rebuild
	var buf bytes.Buffer
	encoder := xml.NewEncoder(&buf)
	encoder.Indent("", "  ")

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}

		if err != nil {
			return "", err
		}

		if err := encoder.EncodeToken(token); err != nil {
			return "", err
		}
	}

	if err := encoder.Flush(); err != nil {
		return "", err
	}

	return xml.Header + buf.String(), nil
}
