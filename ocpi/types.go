// Package ocpi holds OCPI 2.2.1 wire types and the mapping from simulator entities onto them.
// It knows nothing about HTTP servers or clients.
package ocpi

import (
	"encoding/json"
	"fmt"
	"time"
)

const (
	Version = "2.2.1"

	StatusCodeSuccess         = 1000
	StatusCodeClientError     = 2000
	StatusCodeInvalidParams   = 2001
	StatusCodeUnknownLocation = 2003
	StatusCodeServerError     = 3000
)

// Timestamp marshals as OCPI's DateTime: UTC, second precision, trailing Z.
type Timestamp time.Time

const timestampLayout = "2006-01-02T15:04:05Z"

func (t Timestamp) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Time(t).UTC().Format(timestampLayout))
}

func (t *Timestamp) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return fmt.Errorf("json.Unmarshal: %w", err)
	}

	parsed, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return fmt.Errorf("time.Parse: %w", err)
	}

	*t = Timestamp(parsed)

	return nil
}

type Response struct {
	Data          any       `json:"data,omitempty"`
	StatusCode    int       `json:"status_code"`
	StatusMessage string    `json:"status_message,omitempty"`
	Timestamp     Timestamp `json:"timestamp"`
}

type VersionInfo struct {
	URL     string `json:"url"`
	Version string `json:"version"`
}

type VersionDetails struct {
	Endpoints []Endpoint `json:"endpoints"`
	Version   string     `json:"version"`
}

type Endpoint struct {
	Identifier string `json:"identifier"`
	Role       string `json:"role"`
	URL        string `json:"url"`
}

type Location struct {
	Address     string      `json:"address"`
	City        string      `json:"city"`
	Coordinates Coordinates `json:"coordinates"`
	Country     string      `json:"country"`
	CountryCode string      `json:"country_code"`
	EVSEs       []EVSE      `json:"evses"`
	ID          string      `json:"id"`
	LastUpdated Timestamp   `json:"last_updated"`
	Name        string      `json:"name"`
	PartyID     string      `json:"party_id"`
	Publish     bool        `json:"publish"`
	TimeZone    string      `json:"time_zone"`
}

type Coordinates struct {
	Latitude  string `json:"latitude"`
	Longitude string `json:"longitude"`
}

type EVSE struct {
	Capabilities []string    `json:"capabilities"`
	Connectors   []Connector `json:"connectors"`
	EVSEID       string      `json:"evse_id"`
	LastUpdated  Timestamp   `json:"last_updated"`
	Status       string      `json:"status"`
	UID          string      `json:"uid"`
}

type Connector struct {
	Format           string    `json:"format"`
	ID               string    `json:"id"`
	LastUpdated      Timestamp `json:"last_updated"`
	MaxAmperage      int       `json:"max_amperage"`
	MaxElectricPower int       `json:"max_electric_power"`
	MaxVoltage       int       `json:"max_voltage"`
	PowerType        string    `json:"power_type"`
	Standard         string    `json:"standard"`
}

type CDRToken struct {
	ContractID  string `json:"contract_id"`
	CountryCode string `json:"country_code"`
	PartyID     string `json:"party_id"`
	Type        string `json:"type"`
	UID         string `json:"uid"`
}

type Price struct {
	ExclVAT float64 `json:"excl_vat"`
}

type Session struct {
	AuthMethod             string     `json:"auth_method"`
	AuthorizationReference string     `json:"authorization_reference,omitempty"`
	CDRToken               CDRToken   `json:"cdr_token"`
	ConnectorID            string     `json:"connector_id"`
	CountryCode            string     `json:"country_code"`
	Currency               string     `json:"currency"`
	EndDateTime            *Timestamp `json:"end_date_time,omitempty"`
	EVSEUID                string     `json:"evse_uid"`
	ID                     string     `json:"id"`
	KWH                    float64    `json:"kwh"`
	LastUpdated            Timestamp  `json:"last_updated"`
	LocationID             string     `json:"location_id"`
	PartyID                string     `json:"party_id"`
	StartDateTime          Timestamp  `json:"start_date_time"`
	Status                 string     `json:"status"`
	TotalCost              *Price     `json:"total_cost,omitempty"`
}

type CDR struct {
	AuthMethod             string           `json:"auth_method"`
	AuthorizationReference string           `json:"authorization_reference,omitempty"`
	CDRLocation            CDRLocation      `json:"cdr_location"`
	CDRToken               CDRToken         `json:"cdr_token"`
	ChargingPeriods        []ChargingPeriod `json:"charging_periods"`
	CountryCode            string           `json:"country_code"`
	Currency               string           `json:"currency"`
	EndDateTime            Timestamp        `json:"end_date_time"`
	ID                     string           `json:"id"`
	LastUpdated            Timestamp        `json:"last_updated"`
	PartyID                string           `json:"party_id"`
	SessionID              string           `json:"session_id"`
	StartDateTime          Timestamp        `json:"start_date_time"`
	TotalCost              Price            `json:"total_cost"`
	TotalEnergy            float64          `json:"total_energy"`
	TotalTime              float64          `json:"total_time"`
}

type CDRLocation struct {
	Address            string      `json:"address"`
	City               string      `json:"city"`
	ConnectorFormat    string      `json:"connector_format"`
	ConnectorID        string      `json:"connector_id"`
	ConnectorPowerType string      `json:"connector_power_type"`
	ConnectorStandard  string      `json:"connector_standard"`
	Coordinates        Coordinates `json:"coordinates"`
	Country            string      `json:"country"`
	EVSEID             string      `json:"evse_id"`
	EVSEUID            string      `json:"evse_uid"`
	ID                 string      `json:"id"`
	Name               string      `json:"name"`
}

type ChargingPeriod struct {
	Dimensions    []CDRDimension `json:"dimensions"`
	StartDateTime Timestamp      `json:"start_date_time"`
}

type CDRDimension struct {
	Type   string  `json:"type"`
	Volume float64 `json:"volume"`
}

type Token struct {
	ContractID  string `json:"contract_id"`
	CountryCode string `json:"country_code"`
	PartyID     string `json:"party_id"`
	Type        string `json:"type"`
	UID         string `json:"uid"`
}

type StartSession struct {
	AuthorizationReference string `json:"authorization_reference,omitempty"`
	ConnectorID            string `json:"connector_id,omitempty"`
	EVSEUID                string `json:"evse_uid,omitempty"`
	LocationID             string `json:"location_id"`
	ResponseURL            string `json:"response_url"`
	Token                  Token  `json:"token"`
}

type StopSession struct {
	ResponseURL string `json:"response_url"`
	SessionID   string `json:"session_id"`
}

type UnlockConnector struct {
	ConnectorID string `json:"connector_id"`
	EVSEUID     string `json:"evse_uid"`
	LocationID  string `json:"location_id"`
	ResponseURL string `json:"response_url"`
}

const (
	CommandResponseAccepted = "ACCEPTED"
	CommandResponseRejected = "REJECTED"
)

// CommandResponse is the synchronous answer to a command.
type CommandResponse struct {
	Message []DisplayText `json:"message,omitempty"`
	Result  string        `json:"result"`
	Timeout int           `json:"timeout"`
}

// CommandResult is POSTed to the command's response_url once the charger has acted.
type CommandResult struct {
	Message []DisplayText `json:"message,omitempty"`
	Result  string        `json:"result"`
}

type DisplayText struct {
	Language string `json:"language"`
	Text     string `json:"text"`
}
