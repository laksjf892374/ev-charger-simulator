package events

import (
	"sync"

	"cposim/entity"
)

type FakeGateway struct {
	CDREvents                []entity.CDR
	ChargerEvents            []entity.Charger
	ChargerRemovedEvents     []entity.Charger
	CommandEvents            []entity.Command
	PublishCDREventErr       error
	PublishChargerEventErr   error
	PublishChargerRemovedErr error
	PublishCommandEventErr   error
	PublishSessionEventErr   error
	PublishSiteEventErr      error
	SessionEvents            []entity.Session
	SiteEvents               []entity.Site
	mu                       sync.Mutex
}

func NewFakeGateway() *FakeGateway {
	return &FakeGateway{}
}

func (g *FakeGateway) PublishCDREvent(cdr entity.CDR) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.CDREvents = append(g.CDREvents, cdr)

	return g.PublishCDREventErr
}

func (g *FakeGateway) PublishChargerEvent(charger entity.Charger) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.ChargerEvents = append(g.ChargerEvents, charger)

	return g.PublishChargerEventErr
}

func (g *FakeGateway) PublishChargerRemovedEvent(charger entity.Charger) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.ChargerRemovedEvents = append(g.ChargerRemovedEvents, charger)

	return g.PublishChargerRemovedErr
}

func (g *FakeGateway) PublishCommandEvent(command entity.Command) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.CommandEvents = append(g.CommandEvents, command)

	return g.PublishCommandEventErr
}

func (g *FakeGateway) PublishSessionEvent(session entity.Session) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.SessionEvents = append(g.SessionEvents, session)

	return g.PublishSessionEventErr
}

func (g *FakeGateway) PublishSiteEvent(site entity.Site) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.SiteEvents = append(g.SiteEvents, site)

	return g.PublishSiteEventErr
}
