# Reference

Everything the [README](../README.md) leaves out: configuration, the three APIs, how chargers can
be made to behave, limits, tests and deployment. For why it is built this way, see
[DESIGN.md](../DESIGN.md).

## Configuration

| env             | default                   | meaning                                                                 |
|-----------------|---------------------------|-------------------------------------------------------------------------|
| `PORT`          | `8080`                    | port to listen on                                                       |
| `SPEED`         | `1`                       | how fast simulated time runs at start-up, as a multiple of real time (max 600). The hosted demo uses `60`. It can be changed at any time with `PUT /api/clock`. |
| `EMSP_BASE_URL` | the bundled mock eMSP     | where OCPI pushes go: the base URL of **your** eMSP's receiver endpoints. Setting it leaves the mock eMSP unmounted. |

## The three APIs

### 1. OCPI 2.2.1, as a CPO — `/ocpi`

What your eMSP integrates against.

| | |
|---|---|
| `GET /ocpi/versions`, `GET /ocpi/2.2.1` | version negotiation |
| `GET /ocpi/cpo/2.2.1/locations[/{location_id}[/{evse_uid}]]` | pull locations and EVSEs |
| `GET /ocpi/cpo/2.2.1/sessions`, `GET /ocpi/cpo/2.2.1/cdrs` | pull sessions and CDRs |
| `POST /ocpi/cpo/2.2.1/commands/{START_SESSION,STOP_SESSION,UNLOCK_CONNECTOR}` | remote commands; the result is POSTed to your `response_url` |

List endpoints support `date_from`, `date_to`, `offset`, `limit` and return `X-Total-Count`,
`X-Limit` and a `Link: …; rel="next"` header. The simulator pushes to
`{EMSP_BASE_URL}/locations/{cc}/{party}/{location}[/{evse}]` (PUT),
`{EMSP_BASE_URL}/sessions/{cc}/{party}/{session}` (PUT) and `{EMSP_BASE_URL}/cdrs` (POST).

Not implemented yet: the credentials handshake and token checks (every request is let through),
real-time token authorization, tariffs.

### 2. Control API — `/api`

Everything a person, or a CI script, can do to the simulated world. The web UI has no private
endpoints; it is one client of this API.

```sh
# what exists, in one call (also: /api/chargers, /api/sessions, /api/cdrs, /api/commands, /api/sites)
curl localhost:8080/api/state

# add a charger (every field optional except site_id; omitted fields get defaults)
curl -X POST localhost:8080/api/chargers -d '{"site_id":"SITE-000001","max_power_kw":150}'

# be the person at the charger: plug-in | unplug | press-stop | inject-fault | clear-fault
curl -X POST localhost:8080/api/chargers/EVSE-000001/actions/plug-in
curl -X POST localhost:8080/api/chargers/EVSE-000001/actions/plug-in \
     -d '{"battery_capacity_kwh":77,"max_power_kw":170,"state_of_charge":0.6}'

# how a charger behaves: GET /api/behaviors lists the kinds and their default params
curl -X PUT localhost:8080/api/chargers/EVSE-000001/behaviors \
     -d '[{"kind":"start_timeout","params":{"timeout_s":20}}]'
curl -X PUT localhost:8080/api/chargers/EVSE-000001/behaviors -d '[]'     # perfectly reliable

# simulated time
curl -X PUT localhost:8080/api/clock -d '{"speed":60}'

# every OCPI exchange in both directions, with a plain-English summary
curl 'localhost:8080/api/trace?since=0'
```

A refused physical action (unplugging a locked cable) is `409`; a rejected configuration is `422`.

Operations:

```sh
curl -X POST localhost:8080/api/reset    # throw the world away and start again from the demo data
curl localhost:8080/healthz              # 503 once the simulation clock has been silent for 5 s
curl localhost:8080/api/metrics          # pushes sent/failed/dropped, commands by result, ticks, requests, gauges
```

Everything the process reports goes to stdout as JSON lines in one format: each request
(successful polls excepted), and any failed tick or failed or dropped push. `/api/state`
carries a `world_id` that changes on reset, so a client holding older state knows to start over.

Guardrails, because an instance may be public and everything is in memory: at most 50 chargers, 20
sites and 8 behaviors per charger; IDs are 1-36 characters of `A-Z a-z 0-9 _ -`; power, price,
vehicle, coordinates, behavior params and clock speed (max 600x) are range-checked; the last 500
completed sessions (with their CDRs) and finished commands are kept and older ones forgotten;
request bodies are capped; the HTTP server has read, write and idle timeouts.

| behavior kind           | what the eMSP sees                                                        |
|-------------------------|---------------------------------------------------------------------------|
| `realistic_reliability` | **default.** a share of starts `FAILED` (`start_failure_rate`, 5%); occasional mid-session fault (`session_faults_per_hour`, 0.02) |
| `reject_start`          | `CommandResponse REJECTED`, no result follows                             |
| `start_fails`           | accepted, then `CommandResult FAILED` after `delay_s`; no session         |
| `start_timeout`         | accepted, then `CommandResult TIMEOUT` after `timeout_s`                  |
| `fault_mid_session`     | session ends after `after_s`, EVSE goes `OUTOFORDER`, cable stays locked, CDR for the partial energy |

Adding one is a small type and one `Register` call in `controller/behavior/builtin.go`; the API and UI pick it
up from the catalog. A charger may have several: they apply in list order, a refusal or a fault is
final, and otherwise the later behavior wins (see the `controller/behavior` package doc).

### 3. Mock eMSP — `/emsp`

A deliberately naive eMSP so the loop can be seen without a real one: an OCPI receiver under
`/emsp/ocpi/2.2.1`, and the "driver's phone" API (`GET /emsp/api/state`,
`POST /emsp/api/{start,stop,unlock,sync}`). It talks to the simulator only over HTTP and believes
whatever it is told, which is what makes a misbehaving CPO visible.

## Tests

- **Unit tests** per package: fakes not mocks, fake clock, no sleeps.
- **Scenarios** (`app/scenarios_*_test.go`): whole user stories through the public APIs of a running
  simulator with the mock eMSP mounted: happy paths, error cases, operator mistakes, a first visit,
  and operations (reset, health, metrics, logging).
- **Invariants** (`app/invariants_test.go`): seeded random walks of hundreds of actions, checking
  after every step what must always be true (a CHARGING charger has a car, a locked cable and
  exactly one active session; energy never goes down or exceeds the battery; one CDR per completed
  session, matching it; never a 5xx). A failure prints the seed's last actions.
- **Concurrency**: many clients acting at once while the simulation ticks, under the race
  detector, with a deadlock timeout, then the same invariants.
- CI runs gofmt, vet and the full race suite before every deploy.

## Deploy

One container, one always-on machine (the simulation has a clock and in-memory state, so it must
not sleep or scale out). Restarting resets the world to the seeded demo data.

```sh
fly launch --copy-config --no-deploy   # once
fly deploy
```

Known limitation of a public deployment: OCPI command results are POSTed to a caller-supplied
`response_url` (that is how OCPI works), and there is no authentication yet, so anyone can make the
server send a small JSON POST to an http(s) URL of their choosing.
