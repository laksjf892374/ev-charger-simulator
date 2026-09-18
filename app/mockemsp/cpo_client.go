package mockemsp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"cposim/ocpi"
)

const (
	cpoModulePath     = "/ocpi/cpo/" + ocpi.Version
	cpoRequestTimeout = 5 * time.Second
	maxLocationPages  = 100
)

var nextLinkPattern = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

// CPOClient is the eMSP's view of a CPO: the OCPI calls it makes to one.
type CPOClient interface {
	PullLocations() ([]ocpi.Location, error)
	SendCommand(kind string, request any) (ocpi.CommandResponse, error)
}

type httpCPOClient struct {
	cpoBaseURL string
	httpClient *http.Client
}

func NewHTTPCPOClient(cpoBaseURL string) CPOClient {
	return httpCPOClient{
		cpoBaseURL: strings.TrimRight(cpoBaseURL, "/"),
		httpClient: &http.Client{Timeout: cpoRequestTimeout},
	}
}

// PullLocations follows OCPI pagination (the Link header) until the last page.
func (c httpCPOClient) PullLocations() ([]ocpi.Location, error) {
	locations := []ocpi.Location{}
	pageURL := c.cpoBaseURL + cpoModulePath + "/locations"

	for page := 0; pageURL != "" && page < maxLocationPages; page++ {
		response, err := c.httpClient.Get(pageURL)
		if err != nil {
			return nil, fmt.Errorf("httpClient.Get: %w", err)
		}

		var pageLocations []ocpi.Location
		refusal, err := decodeEnvelope(response, &pageLocations)
		if err != nil {
			return nil, fmt.Errorf("decodeEnvelope: %w", err)
		}

		if refusal != "" {
			return nil, fmt.Errorf("CPO refused: %s", refusal)
		}

		locations = append(locations, pageLocations...)

		pageURL = ""
		if match := nextLinkPattern.FindStringSubmatch(response.Header.Get("Link")); match != nil {
			pageURL = match[1]
		}
	}

	return locations, nil
}

// decodeEnvelope unwraps an OCPI response. A refusal is the CPO understanding the request and
// saying no (an OCPI status other than 1000); an error is not getting a usable answer at all.
func decodeEnvelope(response *http.Response, data any) (refusal string, err error) {
	defer response.Body.Close()

	var envelope struct {
		Data          json.RawMessage `json:"data"`
		StatusCode    int             `json:"status_code"`
		StatusMessage string          `json:"status_message"`
	}
	decoder := json.NewDecoder(response.Body)
	if err := decoder.Decode(&envelope); err != nil {
		return "", fmt.Errorf("decoder.Decode: %w", err)
	}

	if envelope.StatusCode != ocpi.StatusCodeSuccess {
		return fmt.Sprintf("OCPI status %d: %s", envelope.StatusCode, envelope.StatusMessage), nil
	}

	if err := json.Unmarshal(envelope.Data, data); err != nil {
		return "", fmt.Errorf("json.Unmarshal: %w", err)
	}

	return "", nil
}

func (c httpCPOClient) SendCommand(kind string, request any) (ocpi.CommandResponse, error) {
	requestBody, err := json.Marshal(request)
	if err != nil {
		return ocpi.CommandResponse{}, fmt.Errorf("json.Marshal: %w", err)
	}

	response, err := c.httpClient.Post(
		c.cpoBaseURL+cpoModulePath+"/commands/"+kind,
		"application/json",
		bytes.NewReader(requestBody),
	)
	if err != nil {
		return ocpi.CommandResponse{}, fmt.Errorf("httpClient.Post: %w", err)
	}

	var commandResponse ocpi.CommandResponse
	refusal, err := decodeEnvelope(response, &commandResponse)
	if err != nil {
		return ocpi.CommandResponse{}, fmt.Errorf("decodeEnvelope: %w", err)
	}

	// To the driver a refused request is an outcome like REJECTED, not a broken connection.
	if refusal != "" {
		return ocpi.CommandResponse{
			Message: []ocpi.DisplayText{{Language: "en", Text: refusal}},
			Result:  CommandResponseRefused,
		}, nil
	}

	return commandResponse, nil
}
