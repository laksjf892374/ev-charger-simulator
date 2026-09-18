package ocpi_test

import (
	"net/http"
	"testing"

	"cposim/entity"
	"cposim/internal/assert"
	"cposim/ocpi"
)

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
