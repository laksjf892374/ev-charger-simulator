package ocpi_test

import (
	"encoding/json"
	"testing"
	"time"

	"cposim/assert"
	"cposim/entity"
	"cposim/ocpi"
)

var (
	validMapper = ocpi.Mapper{
		CountryCode: "US",
		Currency:    "USD",
		PartyID:     "SIM",
		TimeZone:    "America/Los_Angeles",
	}
	validStartedAt = time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)
)

func TestEVSEStatus(t *testing.T) {
	t.Run("maps every charger state onto an OCPI status", func(t *testing.T) {
		// When / Then
		assert.Equal(t, ocpi.EVSEStatus(entity.ChargerStateAvailable), "AVAILABLE")
		assert.Equal(t, ocpi.EVSEStatus(entity.ChargerStatePreparing), "BLOCKED")
		assert.Equal(t, ocpi.EVSEStatus(entity.ChargerStateCharging), "CHARGING")
		assert.Equal(t, ocpi.EVSEStatus(entity.ChargerStateFinishing), "BLOCKED")
		assert.Equal(t, ocpi.EVSEStatus(entity.ChargerStateFaulted), "OUTOFORDER")
	})
}

func TestLocation(t *testing.T) {
	t.Run("nests the site's chargers as EVSEs and takes the newest update time", func(t *testing.T) {
		// Given
		site := entity.Site{Latitude: 37.8044, Longitude: -122.2712, Name: "Oakland Hub", SiteID: "SITE-1", UpdatedAt: validStartedAt}
		chargers := []entity.Charger{{
			ChargerID:  "EVSE-000001",
			MaxPowerKW: 150,
			State:      entity.ChargerStateCharging,
			UpdatedAt:  validStartedAt.Add(time.Minute),
		}}

		// When
		location := validMapper.Location(site, chargers)

		// Then
		assert.Equal(t, location.ID, "SITE-1")
		assert.Equal(t, location.Coordinates, ocpi.Coordinates{Latitude: "37.804400", Longitude: "-122.271200"})
		assert.Equal(t, location.LastUpdated, ocpi.Timestamp(validStartedAt.Add(time.Minute)))
		evseCount := len(location.EVSEs)
		assert.Equal(t, evseCount, 1)
		assert.Equal(t, location.EVSEs[0].UID, "EVSE-000001")
		assert.Equal(t, location.EVSEs[0].EVSEID, "US*SIM*EEVSE000001")
		assert.Equal(t, location.EVSEs[0].Status, "CHARGING")
		assert.Equal(t, location.EVSEs[0].Connectors[0].PowerType, "DC")
		assert.Equal(t, location.EVSEs[0].Connectors[0].MaxElectricPower, 150000)
	})
}

func TestSession(t *testing.T) {
	t.Run("maps an active session without an end time, copying its running cost as it is", func(t *testing.T) {
		// Given
		session := entity.Session{
			ChargerID:          "EVSE-000001",
			EnergyDeliveredKWH: 10.12345,
			PricePerKWH:        0.5,
			SessionID:          "SES-000001",
			SiteID:             "SITE-1",
			StartedAt:          validStartedAt,
			State:              entity.SessionStateActive,
			Token:              entity.Token{UID: "TOKEN-1"},
			TotalCost:          4.2,
		}

		// When
		mapped := validMapper.Session(session)

		// Then
		assert.Equal(t, mapped.Status, "ACTIVE")
		assert.Equal(t, mapped.KWH, 10.123)
		// deliberately not energy x price: the mapper must not price anything itself
		assert.Equal(t, mapped.TotalCost.ExclVAT, 4.2)
		assert.Equal(t, mapped.EndDateTime == nil, true)
		assert.Equal(t, mapped.CDRToken.UID, "TOKEN-1")
		assert.Equal(t, mapped.EVSEUID, "EVSE-000001")
		assert.Equal(t, mapped.LocationID, "SITE-1")
	})
}

func TestCDR(t *testing.T) {
	t.Run("maps totals and a single charging period", func(t *testing.T) {
		// Given
		cdr := entity.CDR{
			CDRID:              "CDR-000001",
			ChargerID:          "EVSE-000001",
			Currency:           "USD",
			EndedAt:            validStartedAt.Add(30 * time.Minute),
			EnergyDeliveredKWH: 25,
			SessionID:          "SES-000001",
			StartedAt:          validStartedAt,
			TotalCost:          12.5,
		}

		// When
		mapped := validMapper.CDR(cdr, entity.Site{Name: "Oakland Hub"}, entity.Charger{MaxPowerKW: 50})

		// Then
		assert.Equal(t, mapped.TotalEnergy, 25.0)
		assert.Equal(t, mapped.TotalTime, 0.5)
		assert.Equal(t, mapped.TotalCost, ocpi.Price{ExclVAT: 12.5})
		assert.Equal(t, mapped.CDRLocation.Name, "Oakland Hub")
		assert.Equal(t, mapped.CDRLocation.ConnectorStandard, "IEC_62196_T2_COMBO")
	})
}

func TestTimestamp(t *testing.T) {
	t.Run("marshals as UTC with second precision and round-trips", func(t *testing.T) {
		// Given
		pacific := time.FixedZone("PST", -8*60*60)
		timestamp := ocpi.Timestamp(time.Date(2026, time.January, 1, 4, 0, 0, 123, pacific))

		// When
		data, err := json.Marshal(timestamp)
		assert.NoError(t, err)
		var parsed ocpi.Timestamp
		err = json.Unmarshal(data, &parsed)

		// Then
		assert.NoError(t, err)
		assert.Equal(t, string(data), `"2026-01-01T12:00:00Z"`)
		assert.Equal(t, time.Time(parsed).Equal(validStartedAt), true)
	})
}
