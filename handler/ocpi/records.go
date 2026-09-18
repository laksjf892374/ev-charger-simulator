// Package ocpi is the inbound half of the OCPI adapter: the CPO-side HTTP endpoints an eMSP calls.
package ocpi

import (
	"fmt"
	"net/http"
	"time"

	"cposim/ocpi"
)

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
