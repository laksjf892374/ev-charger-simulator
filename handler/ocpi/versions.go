// Package ocpi is the inbound half of the OCPI adapter: the CPO-side HTTP endpoints an eMSP calls.
package ocpi

import (
	"net/http"

	"cposim/ocpi"
)

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
