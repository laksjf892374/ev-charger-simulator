# CPO Simulator — design

A simulated Charge Point Operator (CPO) that speaks OCPI 2.2.1, so an eMSP (the company behind a
driver's charging app) can test its integration without a partner sandbox or physical chargers —
both the everyday flow and the failure paths, which are the hard ones to reproduce against a real
CPO.

## Who it is for

1. **Primary: an eMSP developer.** They point their backend at the simulator, create chargers,
   make them misbehave, and drive scenarios by hand or from CI. The architecture is built around
   this user: *the control API is the product; the UI is just one client of it.*
2. **Secondary: a newcomer with no EV background.** They need to understand the domain and see the
   tool work in five minutes. The page they land on is built for them: one plain sentence, three
   guided steps, no jargon, and everything else hidden until asked for. Everything that exists
   only for them (the mock eMSP, the page's explanations) sits at the edge of the architecture:
   delete it and the product still works.

## Requirements

- R1 Add and remove chargers at runtime.
- R2 Configure each charger's normal behaviour (power, price) and its reliability, from "about as
  reliable as a real one" (the default) to specific, reproducible failures. Adding a new vector of
  configuration must be cheap.
- R3 Act as a person standing at the charger: plug in, unplug, press stop, break it, fix it.
- R4 Behave as a CPO over real OCPI 2.2.1: locations, sessions, CDRs, commands
  (`START_SESSION`, `STOP_SESSION`, `UNLOCK_CONNECTOR`), both pull and push.
- R5 Simulated time with a speed control; fully deterministic in tests.
- R6 A bundled mock eMSP + explainer so the tool is self-contained and understandable.

Out of scope for v1 (each has a seam, none has code): driver authorization (RFID / real-time token
auth), OCPI credentials handshake and token checks, tariffs module, delivery faults on the
CPO→eMSP link (duplicate/delayed/dropped pushes), persistence, per-tester sandboxes, rate limiting.

## Architecture

```
  handler/ocpi ──┐                                   ┌── OCPI push gateway (implements events.Gateway)
  handler/api  ──┴─▶ controller/command              │
                          │                          │
                          ▼                          │
                     controller/charger ─────────────┤ events
                          │                          │
                          ▼                          │
                     controller/session ─────────────┘
                          │
        repository/* · gateway/{clock,identifier,metrics,random,scheduler,trace}
                          controller/behavior (pure policy the controllers consult)
```

- **The core knows nothing about OCPI or HTTP.** Controllers take plain inputs and announce
  changes through `events.Gateway`. OCPI is an adapter on both sides: handlers inbound, an
  `events.Gateway` implementation outbound. Another protocol version (or OICP) is a new adapter.
- **Three controllers, one direction.** `command` (async remote-command lifecycle) → `charger`
  (hardware simulation and state machine) → `session` (bookkeeping, CDRs). Locks are only ever
  taken in that order.
- **Simulated time is a gateway.** `clock.Gateway.Now()` is wall time × speed. Controllers expose a
  synchronous `Tick()` that advances by the simulated time elapsed; the scheduler calls it for
  real, tests call it directly after moving a fake clock. No sleeps anywhere.
- **Charger behaviors are a registry of small types** (`controller/behavior/`). A behavior implements whichever hooks it
  needs (`StartInterceptor`, `TickInterceptor`), is registered by name with default params, and is
  configured per charger as JSON. The catalog is served by the API and rendered by the UI, so a
  new behavior is one type and one `Register` call, with no API or UI change. When a charger has
  several, they apply in list order: a refusal or a fault is final, otherwise the later one wins.
- **A session is priced in one function.** The session carries its running cost; the CDR, the
  OCPI mapper and the UI copy it. Real tariffs would replace that function and nothing else.
- **Remote commands are two-phase,** like OCPI: a synchronous accept/reject, then a result that
  arrives later. The core carries an opaque `CallbackReference` so adapters stay stateless.

### Charger state machine

```
AVAILABLE ──plug in──▶ PREPARING ──remote start──▶ CHARGING
    ▲                      │                          │ remote stop · stop button · vehicle full
    └───────unplug─────────┴◀──────── FINISHING ◀─────┘
any ──fault──▶ FAULTED ──clear──▶ AVAILABLE | PREPARING
```

The connector is locked while charging. A fault mid-session ends the session (a CDR is still
issued) and leaves the cable locked until `UNLOCK_CONNECTOR` or the fault is cleared. A remote
start on an unplugged charger waits for plug-in until `StartTimeout`, then resolves `TIMEOUT`.

### Built-in behaviors (all CPO-side)

New chargers are `realistic_reliability` unless they say otherwise: the everyday case is a
charger that mostly works, and an eMSP that is only ever tested against perfect chargers or
certain failures has not been tested against the real thing. Randomness is injected
(`gateway/random`): controllers draw one roll per start attempt and one per charger per tick and
hand it to the behavior, so behaviors stay stateless and every test is deterministic.

| kind                | what the eMSP sees                                              |
|---------------------|-----------------------------------------------------------------|
| `realistic_reliability` | **default.** 5% of starts `FAILED`; 0.02 mid-session faults per hour; both configurable, 0 = perfect |
| `reject_start`      | `CommandResponse REJECTED`, no result follows                   |
| `start_fails`       | accepted, then `CommandResult FAILED` after a delay; no session |
| `start_timeout`     | accepted, then `CommandResult TIMEOUT` after a long wait        |
| `fault_mid_session` | session ends early, EVSE goes `OUTOFORDER`, CDR for partial energy |

## Operating it

- **Reset.** `app` separates the *simulator* (HTTP entry point, tick, logging, metrics) from a
  *world* (one complete simulation). `POST /api/reset` builds a new world, makes it current, seeds
  it, and lets the old one go. `world_id` tells clients their state is stale. Per-tester sandboxes
  would be a map of worlds behind the same entry point.
- **One log, one format.** Requests, failed ticks and failed or dropped pushes are JSON lines on
  stdout; counters and gauges are at `/api/metrics`; every OCPI exchange is at `/api/trace`.
- **Health means the simulation, not the server.** `/healthz` fails when the tick has been silent
  for five seconds; a process whose clock had died would otherwise answer requests forever. A
  panicking tick is recovered, logged and counted, not fatal.
- **Bounded by construction.** Everything a caller can create is capped or forgotten oldest-first,
  everything a caller can configure is range-checked, and a slow or dead eMSP can only fill a
  queue whose overflow is dropped and counted: it cannot stall the simulation.
- **Tested against sequences nobody thought of.** Alongside unit tests and scripted scenarios, a
  seeded random walk checks the world's invariants after every step, and a stress test does the
  same with concurrent clients under the race detector.

## Deployment

One Go binary, one container, one URL (`SPEED=60` on the hosted demo, so a charge can be watched;
real time otherwise). The mock eMSP is mounted in the same process but is a
separate package that talks to the CPO over HTTP loopback and is known to it only as a configured
push URL — splitting it into its own deployment is a second `main` and two URLs. The UI is static
files embedded in the binary; there is no frontend server or build step.
