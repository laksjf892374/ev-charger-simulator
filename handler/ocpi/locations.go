package ocpi

import (
	"fmt"
	"net/http"
	"time"

	"cposim/entity"
	"cposim/ocpi"
)

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
