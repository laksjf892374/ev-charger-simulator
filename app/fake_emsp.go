package app

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"
)

const fakeEMSPTimeout = time.Second

type FakeEMSPRequest struct {
	Body   string
	Method string
	Path   string
}

// FakeEMSP is an HTTP server that accepts and records whatever the simulator pushes to it.
type FakeEMSP struct {
	URL      string
	mu       sync.Mutex
	received chan struct{}
	requests []FakeEMSPRequest
	server   *httptest.Server
}

func NewFakeEMSP() *FakeEMSP {
	fakeEMSP := &FakeEMSP{
		received: make(chan struct{}, 1024),
	}

	fakeEMSP.server = httptest.NewServer(http.HandlerFunc(fakeEMSP.record))
	fakeEMSP.URL = fakeEMSP.server.URL

	return fakeEMSP
}

func (e *FakeEMSP) record(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)

	e.mu.Lock()
	e.requests = append(e.requests, FakeEMSPRequest{
		Body:   string(body),
		Method: r.Method,
		Path:   r.URL.Path,
	})
	e.mu.Unlock()

	e.received <- struct{}{}

	io.WriteString(w, `{"status_code": 1000}`)
}

func (e *FakeEMSP) Close() {
	e.server.Close()
}

// AwaitRequest blocks until a request whose path ends with pathSuffix and whose body contains
// bodySubstring has been received, and returns it.
func (e *FakeEMSP) AwaitRequest(pathSuffix string, bodySubstring string) (FakeEMSPRequest, error) {
	deadline := time.After(fakeEMSPTimeout)

	for {
		if request, ok := e.find(pathSuffix, bodySubstring); ok {
			return request, nil
		}

		select {
		case <-e.received:
		case <-deadline:
			return FakeEMSPRequest{}, fmt.Errorf(
				"no request to …%s containing %q within %v; received: %v",
				pathSuffix, bodySubstring, fakeEMSPTimeout, e.Requests(),
			)
		}
	}
}

func (e *FakeEMSP) find(pathSuffix string, bodySubstring string) (FakeEMSPRequest, bool) {
	for _, request := range e.Requests() {
		if strings.HasSuffix(request.Path, pathSuffix) && strings.Contains(request.Body, bodySubstring) {
			return request, true
		}
	}

	return FakeEMSPRequest{}, false
}

func (e *FakeEMSP) Requests() []FakeEMSPRequest {
	e.mu.Lock()
	defer e.mu.Unlock()

	return append([]FakeEMSPRequest{}, e.requests...)
}
