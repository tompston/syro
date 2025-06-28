package syro

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"maps"
	"net/http"
)

type Request struct {
	Method            string
	URL               string
	Headers           map[string]string
	Body              []byte
	IgnoreStatusCodes bool
	TLSClientConfig   *tls.Config
}

func NewRequest(method, url string) *Request {
	return &Request{
		Method:  method,
		URL:     url,
		Headers: make(map[string]string),
	}
}

func (r *Request) WithHeaders(headers map[string]string) *Request {
	maps.Copy(r.Headers, headers)
	return r
}

func (r *Request) WithHeader(key, value string) *Request {
	if r.Headers == nil {
		r.Headers = make(map[string]string)
	}
	r.Headers[key] = value
	return r
}

func (r *Request) WithBody(body []byte) *Request {
	r.Body = body
	return r
}

func (r *Request) WithIgnoreStatusCodes(ignore bool) *Request {
	r.IgnoreStatusCodes = ignore
	return r
}

func (r *Request) WithTLSClientConfig(tlsConfig *tls.Config) *Request {
	r.TLSClientConfig = tlsConfig
	return r
}

type Response struct {
	Body       []byte
	Headers    http.Header
	StatusCode int
	RequestURL string // The URL that was requested
}

func (r *Request) Fetch() (*Response, error) {

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

	req, err := http.NewRequest(r.Method, r.URL, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}

	for k, v := range r.Headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{}
	if r.TLSClientConfig != nil {
		client.Transport = &http.Transport{TLSClientConfig: r.TLSClientConfig}
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error fetching %v : %v", r.URL, err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body when fetching %v, status %v: %v, error: ", url, res.Status, err)
	}

	if r.IgnoreStatusCodes {
		return &Response{body, res.Header, res.StatusCode, r.URL}, nil
	}

	if res.StatusCode != 200 && res.StatusCode != 201 && res.StatusCode != 202 {
		bodyStr := ""
		if body != nil {
			bodyUpTo := len(body)
			if bodyUpTo > 1000 {
				bodyUpTo = 1000 // limit to 1000 characters
			}

			// checked, if body contains wrong char codes, code will still work
			// if body is nil it will work too
			bodyStr = string(body[:bodyUpTo])
		}

		return nil, fmt.Errorf("response did not return status in 200 group while requesting %v, status: %v, body: %v", url, res.Status, bodyStr)
	}

	if body == nil {
		return nil, fmt.Errorf("response returned empty body while requesting %v", url)
	}

	return &Response{body, res.Header, res.StatusCode, r.URL}, err
}
