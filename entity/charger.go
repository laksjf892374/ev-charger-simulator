package entity

import (
	"encoding/json"
	"time"
)

type ChargerState string

const (
	ChargerStateAvailable ChargerState = "AVAILABLE"
	ChargerStateCharging  ChargerState = "CHARGING"
	ChargerStateFaulted   ChargerState = "FAULTED"
	ChargerStateFinishing ChargerState = "FINISHING"
	ChargerStatePreparing ChargerState = "PREPARING"
)

type BehaviorSpec struct {
	Kind   string          `json:"kind"`
	Params json.RawMessage `json:"params,omitempty"`
}

type Charger struct {
	Behaviors       []BehaviorSpec `json:"behaviors"`
	ChargerID       string         `json:"charger_id"`
	ConnectorLocked bool           `json:"connector_locked"`
	MaxPowerKW      float64        `json:"max_power_kw"`
	PricePerKWH     float64        `json:"price_per_kwh"`
	SessionID       string         `json:"session_id,omitempty"`
	SiteID          string         `json:"site_id"`
	State           ChargerState   `json:"state"`
	UpdatedAt       time.Time      `json:"updated_at"`
	// nil when no cable is connected. StateOfCharge is as of plug-in or the end of the last
	// session; during a session the live value is derived from the session's energy.
	Vehicle *Vehicle `json:"vehicle,omitempty"`
}

type Site struct {
	Address string `json:"address"`
	City    string `json:"city"`
	// ISO 3166-1 alpha-3, as OCPI's Location.country wants it.
	Country   string    `json:"country"`
	Latitude  float64   `json:"latitude"`
	Longitude float64   `json:"longitude"`
	Name      string    `json:"name"`
	SiteID    string    `json:"site_id"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Vehicle struct {
	BatteryCapacityKWH float64 `json:"battery_capacity_kwh"`
	MaxPowerKW         float64 `json:"max_power_kw"`
	StateOfCharge      float64 `json:"state_of_charge"`
}
