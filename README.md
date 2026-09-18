# EV charging simulator

A pretend EV charging network you can drive from a browser or an API. Companies that build
charging apps normally need a partner's test environment and a physical charger to try their
software end to end; this stands in for both.

**Try it: https://ev-charger-simulator.fly.dev**

1. **Plug a car in** at Charger 1 (left).
2. **Start charging** from the phone (right), and watch the battery fill.
3. **Stop charging**, and see the receipt arrive on the phone.

The page highlights the next button to press. That is the whole loop. Everything else is hidden
until you ask for it: *Simulator controls* (top right) lets you add chargers, make them unreliable
or break them, and change how fast time runs; *What's really going on?* (bottom) explains the two
companies involved and shows every message they exchange.

## Run it yourself

```sh
go run .        # http://localhost:8080
```

Go 1.26, standard library only. No database, no build step.

## If you build charging apps

The simulator is a Charge Point Operator speaking OCPI 2.2.1. Point it at your own backend with
`EMSP_BASE_URL=https://your-emsp/ocpi/2.2.1 go run .` and script it from CI.

One caveat, stated plainly: partner authentication is not implemented yet. The simulator accepts
any caller, and its pushes carry no `Authorization` token or OCPI request-ID headers, so a backend
that enforces those will reject them until that is added. Everything else in the charging flow is real OCPI.

```sh
curl -X POST localhost:8080/api/chargers/EVSE-000001/actions/plug-in
curl -X PUT  localhost:8080/api/chargers/EVSE-000001/behaviors -d '[{"kind":"start_timeout"}]'
```

## More

- [docs/REFERENCE.md](docs/REFERENCE.md): the APIs, charger behaviors, limits, tests, deployment
- [DESIGN.md](DESIGN.md): why it is built the way it is
- [CLAUDE.md](CLAUDE.md): code conventions
