package syro

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"maps"
	"net/http"
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
}

func NewRequest(method, url string) *Request {
	return &Request{
		Method:  method,
		URL:     url,
		Headers: make(map[string]string),
		client:  &http.Client{},
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

func (r *Request) WithHeader(key, value string) *Request {
	r.Headers[key] = value
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

func (r *Request) WithClient(client *http.Client) *Request {
	if client != nil {
		r.client = client
	}
	return r
}

type Response struct {
	Body       []byte
	Header     http.Header
	StatusCode int
	RequestURL string // The URL that was requested
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

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body when fetching %v, status %v: %v, error: ", url, res.Status, err)
	}

	if r.ignoreStatusCodes {
		return &Response{body, res.Header, res.StatusCode, r.URL}, nil
	}

	if res.StatusCode != 200 && res.StatusCode != 201 && res.StatusCode != 202 {
		bodyStr := ""
		if body != nil {
			bodyUpTo := min(len(body), 1000) // limit to x characters
			bodyStr = string(body[:bodyUpTo])
		}

		return nil, fmt.Errorf("response did not return status in 200 group while requesting %v, status: %v, body: %v", url, res.Status, bodyStr)
	}

	if body == nil {
		return nil, fmt.Errorf("response returned empty body while requesting %v", url)
	}

	return &Response{body, res.Header, res.StatusCode, r.URL}, err
}
