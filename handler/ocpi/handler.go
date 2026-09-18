// Package ocpi is the inbound half of the OCPI adapter: the CPO-side HTTP endpoints an eMSP calls.
package ocpi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"cposim/controller/charger"
	"cposim/controller/command"
	"cposim/controller/session"
	"cposim/entity"
	"cposim/gateway/clock"
	"cposim/gateway/trace"
	"cposim/ocpi"
)

const (
	BasePath   = "/ocpi"
	modulePath = BasePath + "/cpo/" + ocpi.Version

	maxRequestBodyBytes = 64 * 1024
)

type Config struct {
	// Advertised to the eMSP as how long it should wait for a command result.
	CommandTimeout time.Duration
	Mapper         ocpi.Mapper
}

type handler struct {
	chargerController charger.Controller
	clockGateway      clock.Gateway
	commandController command.Controller
	config            Config
	sessionController session.Controller
	traceGateway      trace.Gateway
}

func NewHandler(
	chargerController charger.Controller,
	clockGateway clock.Gateway,
	commandController command.Controller,
	config Config,
	sessionController session.Controller,
	traceGateway trace.Gateway,
) http.Handler {
	h := handler{
		chargerController: chargerController,
		clockGateway:      clockGateway,
		commandController: commandController,
		config:            config,
		sessionController: sessionController,
		traceGateway:      traceGateway,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET "+BasePath+"/versions", h.getVersions)
	mux.HandleFunc("GET "+BasePath+"/"+ocpi.Version, h.getVersionDetails)
	mux.HandleFunc("GET "+modulePath+"/locations", h.listLocations)
	mux.HandleFunc("GET "+modulePath+"/locations/{location_id}", h.getLocation)
	mux.HandleFunc("GET "+modulePath+"/locations/{location_id}/{evse_uid}", h.getEVSE)
	mux.HandleFunc("GET "+modulePath+"/sessions", h.listSessions)
	mux.HandleFunc("GET "+modulePath+"/cdrs", h.listCDRs)
	mux.HandleFunc("POST "+modulePath+"/commands/{command_type}", h.postCommand)

	return h.traced(authenticated(mux))
}

// authenticated is where OCPI's "Authorization: Token …" check belongs. Credentials are out of
// scope for now, so every request is let through.
func authenticated(next http.Handler) http.Handler {
	return next
}

func (h handler) getVersions(w http.ResponseWriter, r *http.Request) {
	describe(r, "eMSP asks which OCPI versions the CPO supports")

	h.respond(w, http.StatusOK, ocpi.StatusCodeSuccess, "", []ocpi.VersionInfo{{
		URL:     baseURL(r) + BasePath + "/" + ocpi.Version,
		Version: ocpi.Version,
	}})
}

func (h handler) getVersionDetails(w http.ResponseWriter, r *http.Request) {
	describe(r, "eMSP asks where the CPO's OCPI %s modules live", ocpi.Version)

	endpoints := []ocpi.Endpoint{}
	for _, module := range []string{"cdrs", "commands", "locations", "sessions"} {
		role := "SENDER"
		if module == "commands" {
			role = "RECEIVER"
		}

		endpoints = append(endpoints, ocpi.Endpoint{
			Identifier: module,
			Role:       role,
			URL:        baseURL(r) + modulePath + "/" + module,
		})
	}

	h.respond(w, http.StatusOK, ocpi.StatusCodeSuccess, "", ocpi.VersionDetails{
		Endpoints: endpoints,
		Version:   ocpi.Version,
	})
}

func baseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}

	return scheme + "://" + r.Host
}

func (h handler) respond(w http.ResponseWriter, httpStatus int, ocpiStatusCode int, message string, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)

	// an encode failure here means the client went away; there is nobody left to tell
	_ = json.NewEncoder(w).Encode(ocpi.Response{
		Data:          data,
		StatusCode:    ocpiStatusCode,
		StatusMessage: message,
		Timestamp:     ocpi.Timestamp(h.clockGateway.Now()),
	})
}

func (h handler) respondServerError(w http.ResponseWriter, err error) {
	h.respond(w, http.StatusInternalServerError, ocpi.StatusCodeServerError, err.Error(), nil)
}

func (h handler) listLocations(w http.ResponseWriter, r *http.Request) {
	describe(r, "eMSP pulls the CPO's list of locations and chargers")

	locations, err := h.locations()
	if err != nil {
		h.respondServerError(w, fmt.Errorf("locations: %w", err))
		return
	}

	h.respondPage(w, r, pageOf(locations, func(location ocpi.Location) time.Time {
		return time.Time(location.LastUpdated)
	}))
}

func (h handler) locations() ([]ocpi.Location, error) {
	sites, err := h.chargerController.ListSites()
	if err != nil {
		return nil, fmt.Errorf("chargerController.ListSites: %w", err)
	}

	chargers, err := h.chargerController.ListChargers()
	if err != nil {
		return nil, fmt.Errorf("chargerController.ListChargers: %w", err)
	}

	chargersBySiteID := map[string][]entity.Charger{}
	for _, siteCharger := range chargers {
		chargersBySiteID[siteCharger.SiteID] = append(chargersBySiteID[siteCharger.SiteID], siteCharger)
	}

	locations := make([]ocpi.Location, 0, len(sites))
	for _, site := range sites {
		locations = append(locations, h.config.Mapper.Location(site, chargersBySiteID[site.SiteID]))
	}

	return locations, nil
}

func (h handler) getLocation(w http.ResponseWriter, r *http.Request) {
	locationID := r.PathValue("location_id")
	describe(r, "eMSP pulls location %s", locationID)

	locations, err := h.locations()
	if err != nil {
		h.respondServerError(w, fmt.Errorf("locations: %w", err))
		return
	}

	for _, location := range locations {
		if location.ID == locationID {
			h.respond(w, http.StatusOK, ocpi.StatusCodeSuccess, "", location)
			return
		}
	}

	h.respond(w, http.StatusNotFound, ocpi.StatusCodeUnknownLocation, fmt.Sprintf("unknown location %q", locationID), nil)
}

func (h handler) getEVSE(w http.ResponseWriter, r *http.Request) {
	locationID, evseUID := r.PathValue("location_id"), r.PathValue("evse_uid")
	describe(r, "eMSP pulls charger %s at location %s", evseUID, locationID)

	evseCharger, err := h.chargerController.GetCharger(evseUID)
	if err != nil || evseCharger.SiteID != locationID {
		h.respond(w, http.StatusNotFound, ocpi.StatusCodeUnknownLocation, fmt.Sprintf("unknown EVSE %q at location %q", evseUID, locationID), nil)
		return
	}

	h.respond(w, http.StatusOK, ocpi.StatusCodeSuccess, "", h.config.Mapper.EVSE(evseCharger))
}

func (h handler) listSessions(w http.ResponseWriter, r *http.Request) {
	describe(r, "eMSP pulls charging sessions (its safety net for missed pushes)")

	sessions, err := h.sessionController.ListSessions()
	if err != nil {
		h.respondServerError(w, fmt.Errorf("sessionController.ListSessions: %w", err))
		return
	}

	mapped := make([]ocpi.Session, 0, len(sessions))
	for _, listedSession := range sessions {
		mapped = append(mapped, h.config.Mapper.Session(listedSession))
	}

	h.respondPage(w, r, pageOf(mapped, func(mappedSession ocpi.Session) time.Time {
		return time.Time(mappedSession.LastUpdated)
	}))
}

func (h handler) listCDRs(w http.ResponseWriter, r *http.Request) {
	describe(r, "eMSP pulls charge detail records (final bills)")

	cdrs, err := h.sessionController.ListCDRs()
	if err != nil {
		h.respondServerError(w, fmt.Errorf("sessionController.ListCDRs: %w", err))
		return
	}

	mapped := make([]ocpi.CDR, 0, len(cdrs))
	for _, cdr := range cdrs {
		// best effort: the site or charger may have been removed since
		site, _ := h.chargerController.GetSite(cdr.SiteID)
		cdrCharger, _ := h.chargerController.GetCharger(cdr.ChargerID)
		mapped = append(mapped, h.config.Mapper.CDR(cdr, site, cdrCharger))
	}

	h.respondPage(w, r, pageOf(mapped, func(mappedCDR ocpi.CDR) time.Time {
		return time.Time(mappedCDR.LastUpdated)
	}))
}
