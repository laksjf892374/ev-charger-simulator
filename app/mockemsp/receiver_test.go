package mockemsp_test

import (
	"net/http"
	"strconv"
	"testing"

	"cposim/internal/assert"
)

func TestReceiver(t *testing.T) {
	t.Run("rejects a push that is not JSON", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		recorder := serve(t, f, http.MethodPost, "/emsp/ocpi/2.2.1/cdrs", "not json")

		// Then
		assert.Equal(t, recorder.Code, http.StatusBadRequest)
	})

	t.Run("answers not found for the result of a command it never sent", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		recorder := serve(t, f, http.MethodPost, "/emsp/ocpi/2.2.1/commands/START_SESSION/REQ-9999", `{"result": "ACCEPTED"}`)

		// Then
		assert.Equal(t, recorder.Code, http.StatusNotFound)
	})

	t.Run("keeps a location's EVSEs when the location is announced again without them", func(t *testing.T) {
		// Given
		f := newFixture(t)
		serve(t, f, http.MethodPut, "/emsp/ocpi/2.2.1/locations/US/SIM/SITE-1/EVSE-1", `{"status": "AVAILABLE"}`)

		// When
		recorder := serve(t, f, http.MethodPut, "/emsp/ocpi/2.2.1/locations/US/SIM/SITE-1", `{"name": "Oakland Hub", "evses": []}`)

		// Then
		assert.Equal(t, recorder.Code, http.StatusOK)
		locations := state(t, f).Locations
		assert.Equal(t, locations[0].Name, "Oakland Hub")
		assert.Equal(t, locations[0].EVSEs[0].UID, "EVSE-1")
	})

	t.Run("believes the latest EVSE status and forgets an EVSE that is REMOVED", func(t *testing.T) {
		// Given
		f := newFixture(t)
		serve(t, f, http.MethodPut, "/emsp/ocpi/2.2.1/locations/US/SIM/SITE-1/EVSE-1", `{"status": "AVAILABLE"}`)
		serve(t, f, http.MethodPut, "/emsp/ocpi/2.2.1/locations/US/SIM/SITE-1/EVSE-2", `{"status": "AVAILABLE"}`)

		// When
		serve(t, f, http.MethodPut, "/emsp/ocpi/2.2.1/locations/US/SIM/SITE-1/EVSE-1", `{"status": "CHARGING"}`)
		serve(t, f, http.MethodPut, "/emsp/ocpi/2.2.1/locations/US/SIM/SITE-1/EVSE-2", `{"status": "REMOVED"}`)

		// Then
		evses := state(t, f).Locations[0].EVSEs
		evseCount := len(evses)
		assert.Equal(t, evseCount, 1)
		assert.Equal(t, evses[0].Status, "CHARGING")
	})

	t.Run("stores sessions by ID and bills every CDR it is sent, even a duplicate", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		serve(t, f, http.MethodPut, "/emsp/ocpi/2.2.1/sessions/US/SIM/SES-1", `{"status": "ACTIVE", "kwh": 1}`)
		serve(t, f, http.MethodPut, "/emsp/ocpi/2.2.1/sessions/US/SIM/SES-1", `{"status": "COMPLETED", "kwh": 5}`)
		serve(t, f, http.MethodPost, "/emsp/ocpi/2.2.1/cdrs", `{"id": "CDR-1"}`)
		serve(t, f, http.MethodPost, "/emsp/ocpi/2.2.1/cdrs", `{"id": "CDR-1"}`)

		// Then
		believed := state(t, f)
		sessionCount := len(believed.Sessions)
		assert.Equal(t, sessionCount, 1)
		assert.Equal(t, believed.Sessions[0].Status, "COMPLETED")
		assert.Equal(t, believed.Sessions[0].KWH, 5.0)
		cdrCount := len(believed.CDRs)
		assert.Equal(t, cdrCount, 2)
	})
}

func TestHistoryIsBounded(t *testing.T) {
	t.Run("keeps only the most recent bills", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		for i := 0; i < 205; i++ {
			serve(t, f, http.MethodPost, "/emsp/ocpi/2.2.1/cdrs", `{"id": "CDR-`+strconv.Itoa(i)+`"}`)
		}

		// Then
		cdrs := state(t, f).CDRs
		cdrCount := len(cdrs)
		assert.Equal(t, cdrCount, 200)
		assert.Equal(t, cdrs[0].ID, "CDR-5")
	})
}
