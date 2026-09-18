package entity

import "time"

type SessionState string

const (
	SessionStateActive    SessionState = "ACTIVE"
	SessionStateCompleted SessionState = "COMPLETED"
)

type StopReason string

const (
	StopReasonFault       StopReason = "FAULT"
	StopReasonRemote      StopReason = "REMOTE"
	StopReasonStopButton  StopReason = "STOP_BUTTON"
	StopReasonVehicleFull StopReason = "VEHICLE_FULL"
)

type CDR struct {
	AuthorizationReference string    `json:"authorization_reference,omitempty"`
	CDRID                  string    `json:"cdr_id"`
	ChargerID              string    `json:"charger_id"`
	CreatedAt              time.Time `json:"created_at"`
	Currency               string    `json:"currency"`
	EndedAt                time.Time `json:"ended_at"`
	EnergyDeliveredKWH     float64   `json:"energy_delivered_kwh"`
	PricePerKWH            float64   `json:"price_per_kwh"`
	SessionID              string    `json:"session_id"`
	SiteID                 string    `json:"site_id"`
	StartedAt              time.Time `json:"started_at"`
	Token                  Token     `json:"token"`
	TotalCost              float64   `json:"total_cost"`
}

type Session struct {
	AuthorizationReference string       `json:"authorization_reference,omitempty"`
	ChargerID              string       `json:"charger_id"`
	EndedAt                *time.Time   `json:"ended_at,omitempty"`
	EnergyDeliveredKWH     float64      `json:"energy_delivered_kwh"`
	PowerKW                float64      `json:"power_kw"`
	PricePerKWH            float64      `json:"price_per_kwh"`
	SessionID              string       `json:"session_id"`
	SiteID                 string       `json:"site_id"`
	StartedAt              time.Time    `json:"started_at"`
	State                  SessionState `json:"state"`
	StopReason             StopReason   `json:"stop_reason,omitempty"`
	Token                  Token        `json:"token"`
	UpdatedAt              time.Time    `json:"updated_at"`
}

// Token identifies the driver, as issued by the eMSP.
type Token struct {
	ContractID  string `json:"contract_id"`
	CountryCode string `json:"country_code"`
	PartyID     string `json:"party_id"`
	Type        string `json:"type"`
	UID         string `json:"uid"`
}
