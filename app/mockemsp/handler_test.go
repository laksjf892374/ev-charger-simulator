package mockemsp_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cposim/app/mockemsp"
	"cposim/internal/assert"
	"cposim/ocpi"
)

var validConfig = mockemsp.Config{
	DriverToken: ocpi.Token{ContractID: "US-EMS-C0001", Type: "APP_USER", UID: "DRIVER-1"},
	SelfBaseURL: "http://emsp.example/",
}

type fixture struct {
	cpoClient *mockemsp.FakeCPOClient
	mock      mockemsp.Mock
}

func newFixture(t *testing.T) fixture {
	t.Helper()

	cpoClient := mockemsp.NewFakeCPOClient()
	cpoClient.SendCommandResult = ocpi.CommandResponse{Result: "ACCEPTED"}

	return fixture{
		cpoClient: cpoClient,
		mock:      mockemsp.NewMock(validConfig, cpoClient),
	}
}

func serve(t *testing.T, f fixture, method string, target string, body string) *httptest.ResponseRecorder {
	t.Helper()

	recorder := httptest.NewRecorder()
	f.mock.ServeHTTP(recorder, httptest.NewRequest(method, target, strings.NewReader(body)))

	return recorder
}

func state(t *testing.T, f fixture) mockemsp.State {
	t.Helper()

	recorder := serve(t, f, http.MethodGet, "/emsp/api/state", "")
	assert.Equal(t, recorder.Code, http.StatusOK)

	var decoded mockemsp.State
	assert.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &decoded))

	return decoded
}
