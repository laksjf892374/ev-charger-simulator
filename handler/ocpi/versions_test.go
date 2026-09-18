package ocpi_test

import (
	"net/http"
	"testing"

	"cposim/internal/assert"
	"cposim/ocpi"
)

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
