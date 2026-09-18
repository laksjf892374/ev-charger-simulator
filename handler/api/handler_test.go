package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cposim/controller/charger"
	"cposim/controller/command"
	"cposim/controller/session"
	"cposim/entity"
	"cposim/gateway/clock"
	"cposim/gateway/metrics"
	"cposim/gateway/trace"
	"cposim/handler/api"
	"cposim/internal/assert"
)

const validChargerID = "EVSE-000001"

type fixture struct {
	chargerController *charger.FakeController
	clockGateway      *clock.FakeGateway
	handler           http.Handler
	metricsGateway    metrics.Gateway
	sessionController *session.FakeController
	traceGateway      *trace.FakeGateway
}

func newFixture(t *testing.T) fixture {
	t.Helper()

	chargerController := charger.NewFakeController()
	chargerController.GetChargerResult = entity.Charger{ChargerID: validChargerID}
	clockGateway := clock.NewFakeGateway()
	metricsGateway := metrics.NewInMemoryGateway()
	sessionController := session.NewFakeController()
	traceGateway := trace.NewFakeGateway()

	return fixture{
		chargerController: chargerController,
		clockGateway:      clockGateway,
		handler: api.NewHandler(
			chargerController,
			clockGateway,
			command.NewFakeController(),
			api.Config{WorldID: "WORLD-1"},
			metricsGateway,
			sessionController,
			traceGateway,
		),
		metricsGateway:    metricsGateway,
		sessionController: sessionController,
		traceGateway:      traceGateway,
	}
}

func serve(t *testing.T, f fixture, method string, target string, body string) *httptest.ResponseRecorder {
	t.Helper()

	recorder := httptest.NewRecorder()
	f.handler.ServeHTTP(recorder, httptest.NewRequest(method, target, strings.NewReader(body)))

	return recorder
}

func decodeBody[T any](t *testing.T, recorder *httptest.ResponseRecorder) T {
	t.Helper()

	var body T
	assert.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))

	return body
}
