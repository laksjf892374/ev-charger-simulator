package ocpi

import (
	"math"
	"strconv"
	"strings"
	"time"

	"cposim/entity"
)

const (
	// Every simulated charger is one EVSE with one connector.
	ConnectorID = "1"

	EVSEStatusAvailable  = "AVAILABLE"
	EVSEStatusBlocked    = "BLOCKED"
	EVSEStatusCharging   = "CHARGING"
	EVSEStatusOutOfOrder = "OUTOFORDER"
	EVSEStatusRemoved    = "REMOVED"

	authMethodCommand = "COMMAND"
	// Chargers above this power are presented as DC fast chargers, the rest as AC.
	maxACPowerKW = 22
)

// Mapper turns simulator entities into OCPI objects on behalf of one CPO party.
type Mapper struct {
	CountryCode string
	Currency    string
	PartyID     string
	TimeZone    string
}

func (m Mapper) Location(site entity.Site, chargers []entity.Charger) Location {
	lastUpdated := site.UpdatedAt
	evses := make([]EVSE, 0, len(chargers))
	for _, charger := range chargers {
		evses = append(evses, m.EVSE(charger))

		if charger.UpdatedAt.After(lastUpdated) {
			lastUpdated = charger.UpdatedAt
		}
	}

	return Location{
		Address:     site.Address,
		City:        site.City,
		Coordinates: coordinates(site),
		Country:     site.CountryCode,
		CountryCode: m.CountryCode,
		EVSEs:       evses,
		ID:          site.SiteID,
		LastUpdated: Timestamp(lastUpdated),
		Name:        site.Name,
		PartyID:     m.PartyID,
		Publish:     true,
		TimeZone:    m.TimeZone,
	}
}

func coordinates(site entity.Site) Coordinates {
	return Coordinates{
		Latitude:  strconv.FormatFloat(site.Latitude, 'f', 6, 64),
		Longitude: strconv.FormatFloat(site.Longitude, 'f', 6, 64),
	}
}

func (m Mapper) EVSE(charger entity.Charger) EVSE {
	return EVSE{
		Capabilities: []string{"REMOTE_START_STOP_CAPABLE", "UNLOCK_CAPABLE"},
		Connectors:   []Connector{connector(charger)},
		EVSEID:       m.evseID(charger.ChargerID),
		LastUpdated:  Timestamp(charger.UpdatedAt),
		Status:       EVSEStatus(charger.State),
		UID:          charger.ChargerID,
	}
}

func (m Mapper) RemovedEVSE(charger entity.Charger) EVSE {
	evse := m.EVSE(charger)
	evse.Status = EVSEStatusRemoved

	return evse
}

func connector(charger entity.Charger) Connector {
	built := Connector{
		Format:           "SOCKET",
		ID:               ConnectorID,
		LastUpdated:      Timestamp(charger.UpdatedAt),
		MaxElectricPower: int(charger.MaxPowerKW * 1000),
		MaxVoltage:       400,
		PowerType:        "AC_3_PHASE",
		Standard:         "IEC_62196_T2",
	}

	if charger.MaxPowerKW > maxACPowerKW {
		built.Format = "CABLE"
		built.PowerType = "DC"
		built.Standard = "IEC_62196_T2_COMBO"
	}

	built.MaxAmperage = built.MaxElectricPower / built.MaxVoltage

	return built
}

// evseID builds the human-facing eMI3 ID, e.g. US*SIM*EEVSE000001.
func (m Mapper) evseID(chargerID string) string {
	alphanumeric := strings.Map(func(r rune) rune {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}

		return -1
	}, chargerID)

	return m.CountryCode + "*" + m.PartyID + "*E" + strings.ToUpper(alphanumeric)
}

// EVSEStatus maps the charger state machine onto OCPI's vocabulary. OCPI has no notion of
// "cable connected but not charging", so both PREPARING and FINISHING are BLOCKED.
func EVSEStatus(state entity.ChargerState) string {
	switch state {
	case entity.ChargerStateAvailable:
		return EVSEStatusAvailable
	case entity.ChargerStateCharging:
		return EVSEStatusCharging
	case entity.ChargerStateFaulted:
		return EVSEStatusOutOfOrder
	}

	return EVSEStatusBlocked
}

func (m Mapper) Session(session entity.Session) Session {
	mapped := Session{
		AuthMethod:             authMethodCommand,
		AuthorizationReference: session.AuthorizationReference,
		CDRToken:               CDRToken(session.Token),
		ConnectorID:            ConnectorID,
		CountryCode:            m.CountryCode,
		Currency:               m.Currency,
		EVSEUID:                session.ChargerID,
		ID:                     session.SessionID,
		KWH:                    roundTo(session.EnergyDeliveredKWH, 3),
		LastUpdated:            Timestamp(session.UpdatedAt),
		LocationID:             session.SiteID,
		PartyID:                m.PartyID,
		StartDateTime:          Timestamp(session.StartedAt),
		Status:                 string(session.State),
		TotalCost:              &Price{ExclVAT: session.TotalCost},
	}

	if session.EndedAt != nil {
		endDateTime := Timestamp(*session.EndedAt)
		mapped.EndDateTime = &endDateTime
	}

	return mapped
}

// CDR maps a charge detail record. site and charger may be zero values if they have since been
// removed; the CDR is still valid, just less descriptive.
func (m Mapper) CDR(cdr entity.CDR, site entity.Site, charger entity.Charger) CDR {
	chargerConnector := connector(charger)

	return CDR{
		AuthMethod:             authMethodCommand,
		AuthorizationReference: cdr.AuthorizationReference,
		CDRLocation: CDRLocation{
			Address:            site.Address,
			City:               site.City,
			ConnectorFormat:    chargerConnector.Format,
			ConnectorID:        ConnectorID,
			ConnectorPowerType: chargerConnector.PowerType,
			ConnectorStandard:  chargerConnector.Standard,
			Coordinates:        coordinates(site),
			Country:            site.CountryCode,
			EVSEID:             m.evseID(cdr.ChargerID),
			EVSEUID:            cdr.ChargerID,
			ID:                 cdr.SiteID,
			Name:               site.Name,
		},
		CDRToken: CDRToken(cdr.Token),
		ChargingPeriods: []ChargingPeriod{{
			Dimensions: []CDRDimension{
				{Type: "ENERGY", Volume: roundTo(cdr.EnergyDeliveredKWH, 3)},
				{Type: "TIME", Volume: hoursBetween(cdr.StartedAt, cdr.EndedAt)},
			},
			StartDateTime: Timestamp(cdr.StartedAt),
		}},
		CountryCode:   m.CountryCode,
		Currency:      cdr.Currency,
		EndDateTime:   Timestamp(cdr.EndedAt),
		ID:            cdr.CDRID,
		LastUpdated:   Timestamp(cdr.CreatedAt),
		PartyID:       m.PartyID,
		SessionID:     cdr.SessionID,
		StartDateTime: Timestamp(cdr.StartedAt),
		TotalCost:     Price{ExclVAT: cdr.TotalCost},
		TotalEnergy:   roundTo(cdr.EnergyDeliveredKWH, 3),
		TotalTime:     hoursBetween(cdr.StartedAt, cdr.EndedAt),
	}
}

func hoursBetween(startedAt time.Time, endedAt time.Time) float64 {
	return roundTo(endedAt.Sub(startedAt).Hours(), 4)
}

func roundTo(value float64, decimals int) float64 {
	scale := math.Pow(10, float64(decimals))

	return math.Round(value*scale) / scale
}

func (m Mapper) CommandResult(command entity.Command) CommandResult {
	result := CommandResult{Result: string(command.Result)}
	if command.Message != "" {
		result.Message = []DisplayText{{Language: "en", Text: command.Message}}
	}

	return result
}

func EntityToken(token Token) entity.Token {
	return entity.Token(token)
}
