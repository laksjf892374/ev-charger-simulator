package session

import (
	"fmt"
	"math"

	"cposim/entity"
)

func (c *controller) createCDR(session entity.Session) error {
	cdr := entity.CDR{
		AuthorizationReference: session.AuthorizationReference,
		CDRID:                  c.identifierGateway.NewID(cdrIDPrefix),
		ChargerID:              session.ChargerID,
		CreatedAt:              *session.EndedAt,
		Currency:               c.config.Currency,
		EndedAt:                *session.EndedAt,
		EnergyDeliveredKWH:     session.EnergyDeliveredKWH,
		PricePerKWH:            session.PricePerKWH,
		SessionID:              session.SessionID,
		SiteID:                 session.SiteID,
		StartedAt:              session.StartedAt,
		Token:                  session.Token,
		TotalCost:              session.TotalCost,
	}

	if err := c.cdrRepository.Upsert(cdr); err != nil {
		return fmt.Errorf("cdrRepository.Upsert: %w", err)
	}

	if err := c.eventsGateway.PublishCDREvent(cdr); err != nil {
		return fmt.Errorf("eventsGateway.PublishCDREvent: %w", err)
	}

	return nil
}

// cost is the one place a session is priced. Today that is energy times the price the charger
// had when the session started; tariffs with time, session or idle components would replace this
// function (most likely with a pricing package) and nothing else.
func cost(session entity.Session) float64 {
	return roundToCents(session.EnergyDeliveredKWH * session.PricePerKWH)
}

func roundToCents(amount float64) float64 {
	return math.Round(amount*100) / 100
}
