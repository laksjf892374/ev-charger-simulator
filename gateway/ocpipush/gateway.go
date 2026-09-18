// Package ocpipush is the outbound half of the OCPI adapter: it implements events.Gateway by
// pushing OCPI objects to an eMSP's receiver endpoints.
package ocpipush

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"cposim/entity"
	"cposim/gateway/events"
	"cposim/gateway/metrics"
	"cposim/gateway/trace"
	"cposim/ocpi"
	chargerrepo "cposim/repository/charger"
	siterepo "cposim/repository/site"
)

type Config struct {
	// Base URL of the eMSP's OCPI receiver endpoints; modules are expected directly beneath it
	// (…/locations, …/sessions, …/cdrs). Command results go to each command's own response_url.
	EMSPBaseURL string
	Mapper      ocpi.Mapper
	QueueSize   int
}

type Gateway interface {
	events.Gateway
	Stop()
}

type pushGateway struct {
	chargerRepository chargerrepo.Repository
	config            Config
	logger            *slog.Logger
	metricsGateway    metrics.Gateway
	queue             chan func() Push
	sender            Sender
	siteRepository    siterepo.Repository
	stop              chan struct{}
	stopOnce          sync.Once
	stopped           chan struct{}
}

func NewGateway(
	chargerRepository chargerrepo.Repository,
	config Config,
	logger *slog.Logger,
	metricsGateway metrics.Gateway,
	sender Sender,
	siteRepository siterepo.Repository,
) (Gateway, error) {
	if config.EMSPBaseURL == "" {
		return nil, fmt.Errorf("eMSP base URL must not be empty")
	}

	if config.QueueSize <= 0 {
		return nil, fmt.Errorf("queue size must be positive: queue size %d", config.QueueSize)
	}

	config.EMSPBaseURL = strings.TrimRight(config.EMSPBaseURL, "/")

	gateway := &pushGateway{
		chargerRepository: chargerRepository,
		config:            config,
		logger:            logger,
		metricsGateway:    metricsGateway,
		queue:             make(chan func() Push, config.QueueSize),
		sender:            sender,
		siteRepository:    siteRepository,
		stop:              make(chan struct{}),
		stopped:           make(chan struct{}),
	}

	go gateway.run()

	return gateway, nil
}

// run delivers pushes one at a time so the eMSP receives them in the order they happened.
func (g *pushGateway) run() {
	defer close(g.stopped)

	for {
		select {
		case <-g.stop:
			return
		case buildPush := <-g.queue:
			g.metricsGateway.Set(metrics.PushQueueDepth, int64(len(g.queue)))

			push := buildPush()
			if err := g.sender.Send(push); err != nil {
				g.metricsGateway.Add(metrics.PushesFailed, 1)
				g.logger.Error("push failed", "method", push.Method, "url", push.URL, "error", err.Error())
				continue
			}

			g.metricsGateway.Add(metrics.PushesSent, 1)
		}
	}
}

// Stop blocks until the delivery goroutine has exited. Pushes still queued are discarded.
func (g *pushGateway) Stop() {
	g.stopOnce.Do(func() { close(g.stop) })
	<-g.stopped
}

// enqueue never blocks: publishers hold controller locks, so a slow or dead eMSP must not be able
// to stall the simulation. When the queue is full the push is dropped and reported, but the
// publisher is not failed: the state change has already happened, and a real eMSP recovers from a
// missed push by pulling.
func (g *pushGateway) enqueue(description string, buildPush func() Push) error {
	select {
	case g.queue <- buildPush:
		g.metricsGateway.Set(metrics.PushQueueDepth, int64(len(g.queue)))
	default:
		g.metricsGateway.Add(metrics.PushesDropped, 1)
		g.logger.Error("push dropped, queue is full", "push", description)
	}

	return nil
}

func (g *pushGateway) PublishCDREvent(cdr entity.CDR) error {
	return g.enqueue("CDR "+cdr.CDRID, func() Push {
		// best effort: a CDR for a since-removed charger is still valid, just less descriptive
		site, _ := g.siteRepository.Get(cdr.SiteID)
		charger, _ := g.chargerRepository.Get(cdr.ChargerID)

		return Push{
			Body:   g.config.Mapper.CDR(cdr, site, charger),
			Method: http.MethodPost,
			Module: trace.ModuleCDRs,
			Summary: fmt.Sprintf(
				"CPO sends the final bill (CDR) for session %s: %.2f kWh, %.2f %s",
				cdr.SessionID, cdr.EnergyDeliveredKWH, cdr.TotalCost, cdr.Currency,
			),
			URL: g.config.EMSPBaseURL + "/cdrs",
		}
	})
}

func (g *pushGateway) PublishChargerEvent(charger entity.Charger) error {
	return g.enqueue("charger "+charger.ChargerID, func() Push {
		evse := g.config.Mapper.EVSE(charger)

		return Push{
			Body:    evse,
			Method:  http.MethodPut,
			Module:  trace.ModuleLocations,
			Summary: fmt.Sprintf("CPO tells the eMSP that charger %s is now %s", charger.ChargerID, evse.Status),
			URL:     g.evseURL(charger),
		}
	})
}

func (g *pushGateway) evseURL(charger entity.Charger) string {
	return g.partyURL("locations") + "/" + charger.SiteID + "/" + charger.ChargerID
}

func (g *pushGateway) partyURL(module string) string {
	return g.config.EMSPBaseURL + "/" + module + "/" + g.config.Mapper.CountryCode + "/" + g.config.Mapper.PartyID
}

func (g *pushGateway) PublishChargerRemovedEvent(charger entity.Charger) error {
	return g.enqueue("charger removal "+charger.ChargerID, func() Push {
		return Push{
			Body:    g.config.Mapper.RemovedEVSE(charger),
			Method:  http.MethodPut,
			Module:  trace.ModuleLocations,
			Summary: fmt.Sprintf("CPO tells the eMSP that charger %s has been REMOVED", charger.ChargerID),
			URL:     g.evseURL(charger),
		}
	})
}

// PublishCommandEvent only pushes resolved commands: the synchronous accept/reject was already
// given in the HTTP response to the command itself.
func (g *pushGateway) PublishCommandEvent(command entity.Command) error {
	if command.State != entity.CommandStateResolved || command.CallbackReference == "" {
		return nil
	}

	return g.enqueue("command result "+command.CommandID, func() Push {
		return Push{
			Body:   g.config.Mapper.CommandResult(command),
			Method: http.MethodPost,
			Module: trace.ModuleCommands,
			Summary: fmt.Sprintf(
				"CPO reports the outcome of %s (%s): %s",
				command.Kind, command.CommandID, command.Result,
			),
			URL: command.CallbackReference,
		}
	})
}

func (g *pushGateway) PublishSessionEvent(session entity.Session) error {
	return g.enqueue("session "+session.SessionID, func() Push {
		return Push{
			Body:   g.config.Mapper.Session(session),
			Method: http.MethodPut,
			Module: trace.ModuleSessions,
			Summary: fmt.Sprintf(
				"CPO updates session %s: %s, %.2f kWh delivered so far",
				session.SessionID, session.State, session.EnergyDeliveredKWH,
			),
			URL: g.partyURL("sessions") + "/" + session.SessionID,
		}
	})
}

func (g *pushGateway) PublishSiteEvent(site entity.Site) error {
	return g.enqueue("site "+site.SiteID, func() Push {
		return Push{
			Body:    g.config.Mapper.Location(site, nil),
			Method:  http.MethodPut,
			Module:  trace.ModuleLocations,
			Summary: fmt.Sprintf("CPO announces a new location %q (%s)", site.Name, site.SiteID),
			URL:     g.partyURL("locations") + "/" + site.SiteID,
		}
	})
}
