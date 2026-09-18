package ocpi_test

import (
	"errors"
	"net/http"
	"testing"

	"cposim/entity"
	"cposim/internal/assert"
	"cposim/ocpi"
)

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
