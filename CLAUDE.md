# CLAUDE.md

CPO simulator: a simulated EV Charge Point Operator that speaks OCPI 2.2.1, so an eMSP developer
can test their integration (especially failure paths) without a partner sandbox or real chargers.
See `DESIGN.md` for the why; this file is the how.

## Commands

```sh
go test -short ./...                     # quick loop: shortens the random-walk and stress tests
go test ./...                            # full test suite
go test -race ./...                      # use this when touching goroutines, the scheduler, or controllers
go test ./controller/charger -run TestTick   # single package / test
go vet ./...
gofmt -l .                               # must print nothing
go run .                                 # serves on :8080 (PORT), mock eMSP mounted unless EMSP_BASE_URL is set
```

Go 1.26, **standard library only**. Do not add third-party modules (no testify, no mock
generators, no routers) — `go.mod` has no `require` block and should stay that way.

## Architecture

Strict one-way layering; each layer only knows the layer below it, through an interface. Handlers
may use any controller (reads come from `charger` and `session`, commands go through `command`);
controllers only call the controller directly below them:

```
main → app (DI root) → handler/* → controller/command → controller/charger → controller/session → gateway/*, repository/*
                                                                                                      ↓
                                                                                                   entity
```

- `entity/` — plain data structs and typed string state constants. No behaviour, no imports of
  other project packages.
- `repository/<name>/` — in-memory storage behind a `Repository` interface (`Delete`, `Get`,
  `List`, `Upsert`), guarded by a `sync.RWMutex`. Stored values are replaced, never mutated in place.
- `gateway/<name>/` — side effects and nondeterminism behind a `Gateway` interface: `clock`
  (simulated time = wall time × speed), `events` (how the core announces changes; protocol
  adapters implement it), `identifier` (sequential IDs), `scheduler` (one goroutine per job on an
  injectable `Ticker`).
- Pricing lives in exactly one function, `cost` in `controller/session`. The session carries the
  result as `TotalCost`; the CDR, the OCPI mapper and the UI copy it and must never recompute it.
  Real tariffs replace that function (probably with a pure `pricing` package, shaped like
  `behavior`) and nothing else.
- `behavior/` — the fault/scenario registry. Several behaviors on one charger apply in list order;
  a refusal or fault is final, everything else is last-writer-wins (package doc, pinned by tests). Stateless types implementing `StartInterceptor`
  and/or `TickInterceptor`, configured per charger as `entity.BehaviorSpec`. Pure logic, so
  controllers call it directly rather than through an interface.
- `controller/session` owns the session lifecycle, energy bookkeeping and CDR creation.
  `controller/charger` owns **charger state** and simulates the hardware (power delivery, vehicle
  state of charge, faults). `controller/command` owns remote commands: synchronous accept/reject,
  then an asynchronous result. Calls only go command → charger → session.
- **The core (everything above) knows nothing about OCPI or HTTP.** OCPI is an adapter in three
  packages: `ocpi/` (2.2.1 wire types + `Mapper` from entities, no HTTP), `handler/ocpi` (inbound
  CPO endpoints, returned as one `http.Handler`), and `gateway/ocpipush` (outbound; implements
  `events.Gateway`). `Publish…` is called under controller locks, so it only enqueues; one worker
  goroutine delivers in order through a `Sender` (the seam for retries or delivery faults).
- `gateway/trace` — in-memory log of OCPI exchanges in both directions, from the CPO's point of
  view, each with a plain-English `Summary`. Inbound is recorded by `handler/ocpi`'s middleware
  (handlers call `describe`), outbound by the HTTP `Sender`.
- `handler/api` — the control API (`/api/...`): everything a person or CI script can do to the
  simulated world. The UI has no private endpoints. A refused physical action is 409, a bad
  request body 400, a rejected configuration 422.
- `handler/web` — the single static page (`static/index.html`, embedded with `go:embed`; no build
  step, no framework). It polls `/api/state` and `/emsp/api/state` once a second and has no
  endpoints of its own. The left column shows ground truth from the control API; the phone shows
  only what the mock eMSP believes. Keep that separation: it is the point of the demo. The page is for someone who has never heard
  of EV roaming: by default it shows one sentence, three steps with the next button highlighted,
  plain names ("Charger 1", "Free") and nothing else. Anything more (behaviors, add/remove, speed,
  reset, technical IDs and states) carries the `adv` class and appears only with *Simulator
  controls* on; explanations and the message log live in the collapsed *What's really going on?*.
  New UI follows the same rule: if a first-time visitor does not need it, it is `adv`.
- `gateway/random` — the only source of randomness. Controllers draw one roll per start attempt
  and one per charger per tick and hand it to behaviors, which never draw their own.
- `gateway/metrics` — process-wide named counters and gauges, served with derived gauges at
  `/api/metrics`. Add a name constant there when adding a metric.
- `app/` — DI root, in two halves. `world.go` builds one complete, self-contained simulation
  (clock, repositories, controllers, OCPI adapter, mock eMSP) and holds the tunable constants and
  limits; `simulator.go` owns what outlives a world: the HTTP entry point, the tick, `/healthz`,
  `/api/reset`, request logging (`logging.go`) and metrics. Reset swaps in a new world *before*
  seeding it, because seeding pushes to the mock eMSP through this same server. Anything new that
  holds simulation state belongs in the world, or reset will not reset it. The `app` tests are end to end: real HTTP, `scheduler.FakeTicker`, a fake
  wall clock, and either `app.FakeEMSP` (records pushes) or the mounted mock eMSP.
  `scenarios_test.go` tells whole user stories with the helpers in `scenario_helpers_test.go`
  (add a scenario there for any new user-visible behaviour); `invariants_test.go` is the seeded
  random walk and the concurrency stress test (add an invariant there for any new state rule, and
  a weighted action for any new way to change the world).
- `mockemsp/` — a deliberately naive eMSP (OCPI receiver + a tiny "driver's app" API under
  `/emsp/...`) so the simulator can be demonstrated without a real one. It is a guest at the edge:
  only `app` and `main` import it, it is handed URLs rather than controllers, and it talks to the
  CPO only over HTTP, like a real eMSP. It may import `ocpi` for wire types, nothing else.
  Setting `EMSP_BASE_URL` points pushes at a real eMSP and leaves the mock unmounted.

State changes always go through the controller's private `updateX` helper, which does
**`Upsert` then `Publish…Event`**. One deliberate exception: `session.RecordSessionProgress`
upserts every tick but publishes only every `SessionUpdateInterval`, because every event becomes
an OCPI push.

Metering: `charger.meter` is the only place energy is computed. Every path that ends a session
outside a tick calls `meterSinceLastTick` first, so a bill never depends on where in the tick
interval the session stopped.

Time: nothing reads the wall clock except `clock.NewScaledGateway`. Controllers expose `Tick()`,
which advances the simulation by however much *simulated* time passed since the last tick. One
tick drives all chargers (no per-session jobs), so a session can end itself from inside a tick.

## Code conventions

**Packages & naming**
- Package name is the domain noun (`charger`, `session`, `events`, `scheduler`, `cli`), so types
  are just `Controller`, `Gateway`, `Repository`, `Handler` and read as `charger.Controller`.
- Because `repository/charger` and `controller/charger` collide, repositories are always imported
  with an alias: `chargerrepo`, `sessionrepo`.
- Descriptive, unabbreviated variable names that repeat the role: `chargerRepository`,
  `eventsGateway`, `sessionController`, `normalizedCommand`, `successfulScan`. Maps are named
  `<value>By<Key>` (`sessionBySessionID`, `cancelFuncsByJobID`).
- Units go in the name: `PowerKW`, `EnergyDeliveredKWH`, `maxPowerJitterPercent`.
- Struct fields, interface methods, const blocks, and constructor parameters are kept in
  **alphabetical order** (a `mu` mutex goes last). Constructor args mirror the struct field order.

**Interfaces & constructors**
- Exported interface + unexported implementation struct + exported constructor returning the
  interface: `NewController(...) Controller`, `NewInMemoryRepository() Repository`,
  `NewPrintGateway(w) Gateway`, `NewTickerGateway(...) Gateway`. Implementation names describe the
  mechanism (`inMemoryRepository`, `printGateway`, `tickerGateway`).
- Value receivers for stateless structs; pointer receivers (and a returned `&impl{}`) only when the
  struct holds a mutex or mutable maps.
- Constructors validate their config and return `(T, error)` when there is something to validate
  (see `session.NewController`); otherwise they return just `T`.
- Nondeterminism is injected, not hidden: simulated time comes from `gateway/clock`, IDs from
  `gateway/identifier`, ticks from `scheduler.NewTickerFunc`.

**Errors**
- Every error from a call is wrapped with the *callee's name* and nothing else:
  `fmt.Errorf("chargerRepository.Get: %w", err)`, `fmt.Errorf("updateSession: %w", err)`.
  This applies even in `main.go`.
- Errors that originate here are lowercase, human-readable sentences with `%q` for IDs and states:
  `charger %q is not available: charger state %q`.
- On error return the zero value explicitly (`entity.Session{}`, `nil`), never a half-filled struct.
- Guard clauses / early returns; validate state before acting. No panics, no sentinel errors, no
  custom error types so far.

**Layout & style**
- Functions are ordered top-down in call order: an exported method is followed by the private
  helpers it uses, not grouped by visibility.
- Multi-argument calls, signatures, and struct literals are split one-argument-per-line with a
  trailing comma once they don't fit on a line.
- Blank line after a guard block and before the final `return`.
- Comments are rare and explain *why* (a non-obvious invariant, a known limitation such as
  `// ideally atomic operation starting here`). No doc comments restating the name.
- Named results only where they document a bare bool: `(shouldQuit bool)`.
- Magic values become named constants (`sessionUpdateFrequency`, `CommandStart`).

**Concurrency**
- Shared state is protected with a mutex locked at the top of the method with `defer Unlock()`.
- Goroutines have an explicit stop channel *and* a stopped channel; `StopScheduledJob` blocks until
  the job goroutine has actually exited, so no callback runs after stop returns. Never call it from
  inside the job's own callback.
- Controller locks are only ever taken top-down (`command` → `charger` → `session`). A lower
  controller never calls a higher one.
- Errors inside a background job can't be returned, so they're written to the injected `out`.

## Testing conventions

- External test packages (`package charger_test`), exercising only the exported API.
- One `TestXxx` per method/function, containing `t.Run` subtests whose names are full behaviour
  sentences: `"returns an error when the charger is already in use"`,
  `"marks the charger in use and starts progress updates for the new session"`.
- Every subtest is structured with `// Given`, `// When`, `// Then` comments (repeated
  `// When` / `// Then` pairs for multi-step flows). Subtests build their own fixtures — no shared
  mutable state, no table-driven tests; error paths come first, the happy path last.
- Assertions use the in-repo `assert` package only: `assert.Equal(t, got, want)` (got first),
  `NotEqual`, `NoError`, `Error`, `Contains`, `NotContains`. They are `t.Fatalf`-based. Compute
  non-comparable things into a local first (`chargerEventCount := len(...)`). Add a new helper to
  `assert/assert.go` (with `t.Helper()`) rather than hand-writing `if got != want`.
- Test helpers take `t *testing.T` first and call `t.Helper()` (`seedCharger`,
  `newSessionController`). Shared valid config lives in `valid…` constants at the top of the file.
- Injected failures use `errors.New("boom")`.

**Fakes, not mocks**
- Every gateway and controller interface has a hand-written `Fake<Type>` in `fake_<name>.go` **next to the production
  implementation, in the non-test package** (so other packages' tests can import it), with a
  `NewFake<Type>()` constructor returning a pointer.
- Fakes are configured and inspected through exported fields: `<Method>Result`, `<Method>Err`,
  `<Method>CalledWith` / `…Calls` / recorded event slices. No expectation DSL.
- Each fake has its own `fake_<name>_test.go` verifying it records calls and returns what was
  configured. Add one when adding a fake.
- Use the real in-memory repository in controller tests (repositories have no fakes yet — they
  cannot fail; add one when an error-path test needs it). `identifier.NewSequentialGateway()` is
  deterministic and is also used for real. Fake the other gateways and collaborating controllers.

**Time and goroutines in tests**
- Never `time.Sleep` and never wait on real tickers. Controllers expose a synchronous `Tick()`;
  tests move `clock.FakeGateway` with `Advance(...)` and call `Tick()` directly. Only the scheduler
  gateway's own tests use `scheduler.FakeTicker.Tick()`.
- Anything that could block has a 1-second safety timeout and returns an `error` (asserted with
  `assert.NoError`) instead of hanging the suite.

## Guardrails

An instance may be public and unauthenticated, and everything is in memory. Whatever a caller can
create must be capped or forgotten (`maxChargers`, `maxSites`, `maxCompletedSessions`,
`maxFinishedCommands` in `app/world.go`; `behavior.MaxBehaviorsPerCharger`; the mock eMSP's
`maxStored…`). Whatever a caller can configure must be range-checked where it enters the core
(`validateSite`, `validateVehicle`, `AddCharger`, `clock.validateSpeed`, `behavior.Validator`).
IDs appear in URL paths and OCPI fields: they must match `validID`.

## Adding things

- **New fault/scenario:** a type in `behavior/builtin.go` implementing the interceptor(s) it needs,
  a `Kind…` constant, one `Register` call with default params, and tests in
  `behavior/behavior_test.go`. No controller, API or UI change is needed.
- **New interception point** (e.g. stop commands): a new `…Interceptor` interface + context struct
  + `Apply…Interceptors` in `behavior/behavior.go`, called from the owning controller.
- **New dependency/collaborator:** interface + unexported impl + constructor + `Fake…` +
  `fake_…_test.go` in its own package, then wire it in `app` only.
- Keep `DESIGN.md` in sync when packages or interface methods change.
