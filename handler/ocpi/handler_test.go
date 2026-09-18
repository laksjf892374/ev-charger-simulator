package ocpi_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cposim/assert"
	"cposim/controller/charger"
	"cposim/controller/command"
	"cposim/controller/session"
	"cposim/entity"
	"cposim/gateway/clock"
	"cposim/gateway/trace"
	ocpihandler "cposim/handler/ocpi"
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

func TestVersions(t *testing.T) {
	t.Run("advertises 2.2.1 with an absolute URL built from the request host", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		recorder, decoded := serve(t, f, http.MethodGet, "/ocpi/versions", "")

		// Then
		assert.Equal(t, recorder.Code, http.StatusOK)
		assert.Equal(t, decoded.StatusCode, 1000)
		assert.Equal(t, decodeData[[]ocpi.VersionInfo](t, decoded), []ocpi.VersionInfo{{
			URL:     "http://example.com/ocpi/2.2.1",
			Version: "2.2.1",
		}})
	})

	t.Run("lists the module endpoints for 2.2.1", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		_, decoded := serve(t, f, http.MethodGet, "/ocpi/2.2.1", "")

		// Then
		details := decodeData[ocpi.VersionDetails](t, decoded)
		endpointCount := len(details.Endpoints)
		assert.Equal(t, endpointCount, 4)
		assert.Equal(t, details.Endpoints[1], ocpi.Endpoint{
			Identifier: "commands",
			Role:       "RECEIVER",
			URL:        "http://example.com/ocpi/cpo/2.2.1/commands",
		})
	})
}

func TestLocations(t *testing.T) {
	t.Run("returns a server error when chargers cannot be listed", func(t *testing.T) {
		// Given
		f := newFixture(t)
		f.chargerController.ListChargersErr = errors.New("boom")

		// When
		recorder, decoded := serve(t, f, http.MethodGet, "/ocpi/cpo/2.2.1/locations", "")

		// Then
		assert.Equal(t, recorder.Code, http.StatusInternalServerError)
		assert.Equal(t, decoded.StatusCode, 3000)
	})

	t.Run("returns unknown location for a location that does not exist", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		recorder, decoded := serve(t, f, http.MethodGet, "/ocpi/cpo/2.2.1/locations/missing", "")

		// Then
		assert.Equal(t, recorder.Code, http.StatusNotFound)
		assert.Equal(t, decoded.StatusCode, 2003)
	})

	t.Run("returns unknown location for an EVSE that belongs to another location", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		recorder, _ := serve(t, f, http.MethodGet, "/ocpi/cpo/2.2.1/locations/SITE-OTHER/"+validChargerID, "")

		// Then
		assert.Equal(t, recorder.Code, http.StatusNotFound)
	})

	t.Run("lists sites as locations with their chargers nested as EVSEs", func(t *testing.T) {
		// Given
		f := newFixture(t)
		f.chargerController.ListSitesResult = []entity.Site{{Name: "Oakland Hub", SiteID: validSiteID}, {SiteID: "SITE-EMPTY"}}
		f.chargerController.ListChargersResult = []entity.Charger{{
			ChargerID: validChargerID,
			SiteID:    validSiteID,
			State:     entity.ChargerStateAvailable,
		}}

		// When
		recorder, decoded := serve(t, f, http.MethodGet, "/ocpi/cpo/2.2.1/locations", "")

		// Then
		assert.Equal(t, recorder.Code, http.StatusOK)
		assert.Equal(t, recorder.Header().Get("X-Total-Count"), "2")
		locations := decodeData[[]ocpi.Location](t, decoded)
		assert.Equal(t, locations[0].Name, "Oakland Hub")
		assert.Equal(t, locations[0].EVSEs[0].UID, validChargerID)
		assert.Equal(t, locations[0].EVSEs[0].Status, "AVAILABLE")
		emptyLocationEVSECount := len(locations[1].EVSEs)
		assert.Equal(t, emptyLocationEVSECount, 0)
	})
}

func TestPagination(t *testing.T) {
	sessionsUpdatedEachMinute := func(f fixture, count int) {
		for i := 0; i < count; i++ {
			f.sessionController.ListSessionsResult = append(f.sessionController.ListSessionsResult, entity.Session{
				SessionID: "SES-" + string(rune('A'+i)),
				UpdatedAt: f.clockGateway.Now().Add(time.Duration(i) * time.Minute),
			})
		}
	}

	t.Run("rejects a limit that is not a positive integer", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		recorder, decoded := serve(t, f, http.MethodGet, "/ocpi/cpo/2.2.1/sessions?limit=0", "")

		// Then
		assert.Equal(t, recorder.Code, http.StatusBadRequest)
		assert.Equal(t, decoded.StatusCode, 2001)
	})

	t.Run("rejects a date_from that is not a timestamp", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		recorder, _ := serve(t, f, http.MethodGet, "/ocpi/cpo/2.2.1/sessions?date_from=yesterday", "")

		// Then
		assert.Equal(t, recorder.Code, http.StatusBadRequest)
	})

	t.Run("returns one page with a Link to the next", func(t *testing.T) {
		// Given
		f := newFixture(t)
		sessionsUpdatedEachMinute(f, 3)

		// When
		recorder, decoded := serve(t, f, http.MethodGet, "/ocpi/cpo/2.2.1/sessions?limit=2", "")

		// Then
		sessions := decodeData[[]ocpi.Session](t, decoded)
		sessionCount := len(sessions)
		assert.Equal(t, sessionCount, 2)
		assert.Equal(t, recorder.Header().Get("X-Total-Count"), "3")
		assert.Equal(t, recorder.Header().Get("X-Limit"), "2")
		assert.Equal(t, recorder.Header().Get("Link"), `<http://example.com/ocpi/cpo/2.2.1/sessions?limit=2&offset=2>; rel="next"`)
	})

	t.Run("omits the Link header on the last page", func(t *testing.T) {
		// Given
		f := newFixture(t)
		sessionsUpdatedEachMinute(f, 3)

		// When
		recorder, decoded := serve(t, f, http.MethodGet, "/ocpi/cpo/2.2.1/sessions?limit=2&offset=2", "")

		// Then
		sessions := decodeData[[]ocpi.Session](t, decoded)
		assert.Equal(t, sessions[0].ID, "SES-C")
		assert.Equal(t, recorder.Header().Get("Link"), "")
	})

	t.Run("filters on last_updated from date_from inclusive to date_to exclusive", func(t *testing.T) {
		// Given
		f := newFixture(t)
		sessionsUpdatedEachMinute(f, 3)
		dateFrom := f.clockGateway.Now().Add(time.Minute).Format(time.RFC3339)
		dateTo := f.clockGateway.Now().Add(2 * time.Minute).Format(time.RFC3339)

		// When
		_, decoded := serve(t, f, http.MethodGet, "/ocpi/cpo/2.2.1/sessions?date_from="+dateFrom+"&date_to="+dateTo, "")

		// Then
		sessions := decodeData[[]ocpi.Session](t, decoded)
		sessionCount := len(sessions)
		assert.Equal(t, sessionCount, 1)
		assert.Equal(t, sessions[0].ID, "SES-B")
	})
}

func TestCDRs(t *testing.T) {
	t.Run("lists CDRs described with their site", func(t *testing.T) {
		// Given
		f := newFixture(t)
		f.sessionController.ListCDRsResult = []entity.CDR{{CDRID: "CDR-000001", SiteID: validSiteID, TotalCost: 12.5}}
		f.chargerController.GetSiteResult = entity.Site{Name: "Oakland Hub", SiteID: validSiteID}

		// When
		_, decoded := serve(t, f, http.MethodGet, "/ocpi/cpo/2.2.1/cdrs", "")

		// Then
		cdrs := decodeData[[]ocpi.CDR](t, decoded)
		assert.Equal(t, cdrs[0].ID, "CDR-000001")
		assert.Equal(t, cdrs[0].TotalCost.ExclVAT, 12.5)
		assert.Equal(t, cdrs[0].CDRLocation.Name, "Oakland Hub")
	})
}

func TestCommands(t *testing.T) {
	validStartSessionBody := `{
		"response_url": "` + validResponseURL + `",
		"token": {"uid": "TOKEN-1", "type": "APP_USER", "contract_id": "US-EMS-C1"},
		"location_id": "` + validSiteID + `",
		"evse_uid": "` + validChargerID + `",
		"authorization_reference": "AUTH-1"
	}`

	t.Run("answers NOT_SUPPORTED for a command the CPO does not implement", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		recorder, decoded := serve(t, f, http.MethodPost, "/ocpi/cpo/2.2.1/commands/RESERVE_NOW", `{}`)

		// Then
		assert.Equal(t, recorder.Code, http.StatusOK)
		assert.Equal(t, decodeData[ocpi.CommandResponse](t, decoded).Result, "NOT_SUPPORTED")
	})

	t.Run("rejects a body that is not JSON", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		recorder, decoded := serve(t, f, http.MethodPost, "/ocpi/cpo/2.2.1/commands/START_SESSION", `not json`)

		// Then
		assert.Equal(t, recorder.Code, http.StatusBadRequest)
		assert.Equal(t, decoded.StatusCode, 2001)
	})

	t.Run("rejects a response_url that is not http(s)", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		recorder, _ := serve(t, f, http.MethodPost, "/ocpi/cpo/2.2.1/commands/STOP_SESSION", `{"response_url": "file:///etc/passwd", "session_id": "SES-1"}`)

		// Then
		assert.Equal(t, recorder.Code, http.StatusBadRequest)
		stopSessionCallCount := len(f.commandController.StopSessionCalledWith)
		assert.Equal(t, stopSessionCallCount, 0)
	})

	t.Run("answers unknown location when the EVSE does not exist", func(t *testing.T) {
		// Given
		f := newFixture(t)
		f.chargerController.GetChargerErr = errors.New("boom")

		// When
		recorder, decoded := serve(t, f, http.MethodPost, "/ocpi/cpo/2.2.1/commands/START_SESSION", validStartSessionBody)

		// Then
		assert.Equal(t, recorder.Code, http.StatusNotFound)
		assert.Equal(t, decoded.StatusCode, 2003)
	})

	t.Run("answers REJECTED with the reason when the command controller rejects the start", func(t *testing.T) {
		// Given
		f := newFixture(t)
		f.commandController.StartSessionResult = entity.Command{
			Message: "charger refused the request",
			State:   entity.CommandStateRejected,
		}

		// When
		_, decoded := serve(t, f, http.MethodPost, "/ocpi/cpo/2.2.1/commands/START_SESSION", validStartSessionBody)

		// Then
		response := decodeData[ocpi.CommandResponse](t, decoded)
		assert.Equal(t, response.Result, "REJECTED")
		assert.Equal(t, response.Message[0].Text, "charger refused the request")
	})

	t.Run("answers UNKNOWN_SESSION when asked to stop a session that does not exist", func(t *testing.T) {
		// Given
		f := newFixture(t)
		f.sessionController.GetSessionErr = errors.New("boom")

		// When
		_, decoded := serve(t, f, http.MethodPost, "/ocpi/cpo/2.2.1/commands/STOP_SESSION", `{"response_url": "`+validResponseURL+`", "session_id": "missing"}`)

		// Then
		assert.Equal(t, decodeData[ocpi.CommandResponse](t, decoded).Result, "UNKNOWN_SESSION")
	})

	t.Run("passes a start to the command controller and answers ACCEPTED with the timeout", func(t *testing.T) {
		// Given
		f := newFixture(t)
		f.commandController.StartSessionResult = entity.Command{State: entity.CommandStatePending}

		// When
		recorder, decoded := serve(t, f, http.MethodPost, "/ocpi/cpo/2.2.1/commands/START_SESSION", validStartSessionBody)

		// Then
		assert.Equal(t, recorder.Code, http.StatusOK)
		assert.Equal(t, decodeData[ocpi.CommandResponse](t, decoded), ocpi.CommandResponse{Result: "ACCEPTED", Timeout: 90})
		assert.Equal(t, f.commandController.StartSessionCalledWith, []command.StartSessionInput{{
			AuthorizationReference: "AUTH-1",
			CallbackReference:      validResponseURL,
			ChargerID:              validChargerID,
			Token:                  entity.Token{ContractID: "US-EMS-C1", Type: "APP_USER", UID: "TOKEN-1"},
		}})
	})

	t.Run("passes a stop and an unlock to the command controller", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		serve(t, f, http.MethodPost, "/ocpi/cpo/2.2.1/commands/STOP_SESSION", `{"response_url": "`+validResponseURL+`", "session_id": "SES-1"}`)
		serve(t, f, http.MethodPost, "/ocpi/cpo/2.2.1/commands/UNLOCK_CONNECTOR", `{"response_url": "`+validResponseURL+`", "location_id": "`+validSiteID+`", "evse_uid": "`+validChargerID+`", "connector_id": "1"}`)

		// Then
		assert.Equal(t, f.commandController.StopSessionCalledWith, []command.StopSessionInput{{
			CallbackReference: validResponseURL,
			SessionID:         "SES-1",
		}})
		assert.Equal(t, f.commandController.UnlockConnectorCalledWith, []command.UnlockConnectorInput{{
			CallbackReference: validResponseURL,
			ChargerID:         validChargerID,
		}})
	})
}

func TestTracing(t *testing.T) {
	t.Run("records the exchange with a plain-English summary of request and answer", func(t *testing.T) {
		// Given
		f := newFixture(t)
		f.commandController.StartSessionResult = entity.Command{State: entity.CommandStatePending}
		body := `{"response_url": "` + validResponseURL + `", "token": {"uid": "TOKEN-1"}, "location_id": "` + validSiteID + `", "evse_uid": "` + validChargerID + `"}`

		// When
		serve(t, f, http.MethodPost, "/ocpi/cpo/2.2.1/commands/START_SESSION", body)

		// Then
		entries := f.traceGateway.Recorded()
		entryCount := len(entries)
		assert.Equal(t, entryCount, 1)
		assert.Equal(t, entries[0].Direction, trace.DirectionInbound)
		assert.Equal(t, entries[0].Method, http.MethodPost)
		assert.Equal(t, entries[0].URL, "/ocpi/cpo/2.2.1/commands/START_SESSION")
		assert.Equal(t, entries[0].StatusCode, http.StatusOK)
		assert.Equal(t, entries[0].RequestBody, body)
		assert.Contains(t, entries[0].ResponseBody, `"result":"ACCEPTED"`)
		assert.Equal(t, entries[0].Summary, "eMSP asks the CPO to start charging on EVSE-000001 for driver token TOKEN-1. CPO answers ACCEPTED: the real outcome will follow as a separate call")
	})

	t.Run("records calls to endpoints that do not exist", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		recorder, _ := serve(t, f, http.MethodGet, "/ocpi/cpo/2.2.1/tariffs", "")

		// Then
		assert.Equal(t, recorder.Code, http.StatusNotFound)
		assert.Contains(t, f.traceGateway.Recorded()[0].Summary, "does not have")
	})
}
