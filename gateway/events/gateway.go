package events

import "cposim/entity"

// Gateway is how the core tells the outside world that something changed. The core does not know
// who is listening; protocol adapters (e.g. the OCPI push gateway) implement this interface.
type Gateway interface {
	PublishCDREvent(cdr entity.CDR) error
	PublishChargerEvent(charger entity.Charger) error
	PublishChargerRemovedEvent(charger entity.Charger) error
	PublishCommandEvent(command entity.Command) error
	PublishSessionEvent(session entity.Session) error
	PublishSiteEvent(site entity.Site) error
}
