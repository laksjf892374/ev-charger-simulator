package ocpi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cposim/controller/charger"
	"cposim/controller/command"
	"cposim/controller/session"
	"cposim/entity"
	"cposim/gateway/clock"
	"cposim/gateway/trace"
	ocpihandler "cposim/handler/ocpi"
	"cposim/internal/assert"
	"cposim/ocpi"
)

const (
	validChargerID   = "EVSE-000001"
	validResponseURL = "https://emsp.example/ocpi/commands/START_SESSION/1"
	validSiteID      = "SITE-000001"
)

var validConfig = ocpihandler.Config{
	CommandTimeout: 90 * time.Second,
	Mapper:         ocpi.Mapper{CountryCode: "US", Currency: "USD", PartyID: "SIM"},
}

type fixture struct {
	chargerController *charger.FakeController
	clockGateway      *clock.FakeGateway
	commandController *command.FakeController
	handler           http.Handler
	sessionController *session.FakeController
	traceGateway      *trace.FakeGateway
}

func newFixture(t *testing.T) fixture {
	t.Helper()

	chargerController := charger.NewFakeController()
	chargerController.GetChargerResult = entity.Charger{ChargerID: validChargerID, SiteID: validSiteID}
	clockGateway := clock.NewFakeGateway()
	commandController := command.NewFakeController()
	sessionController := session.NewFakeController()
	traceGateway := trace.NewFakeGateway()

	return fixture{
		chargerController: chargerController,
		clockGateway:      clockGateway,
		commandController: commandController,
		handler: ocpihandler.NewHandler(
			chargerController,
			clockGateway,
			commandController,
			validConfig,
			sessionController,
			traceGateway,
		),
		sessionController: sessionController,
		traceGateway:      traceGateway,
	}
}

type envelope struct {
	Data          json.RawMessage `json:"data"`
	StatusCode    int             `json:"status_code"`
	StatusMessage string          `json:"status_message"`
}

func serve(t *testing.T, f fixture, method string, target string, body string) (*httptest.ResponseRecorder, envelope) {
	t.Helper()

	recorder := httptest.NewRecorder()
	f.handler.ServeHTTP(recorder, httptest.NewRequest(method, target, strings.NewReader(body)))

	var decoded envelope
	if recorder.Body.Len() > 0 && strings.HasPrefix(recorder.Header().Get("Content-Type"), "application/json") {
		assert.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &decoded))
	}

	return recorder, decoded
}

func decodeData[T any](t *testing.T, decoded envelope) T {
	t.Helper()

	var data T
	assert.NoError(t, json.Unmarshal(decoded.Data, &data))

	return data
}
