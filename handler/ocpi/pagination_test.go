package ocpi_test

import (
	"net/http"
	"testing"
	"time"

	"cposim/entity"
	"cposim/internal/assert"
	"cposim/ocpi"
)

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
