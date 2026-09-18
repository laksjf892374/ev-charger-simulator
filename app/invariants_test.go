package app_test

import (
	"fmt"
	"math/rand"
	"net/http"
	"sync"
	"testing"
	"time"

	"cposim/internal/assert"
)

// No script can list every order in which people press buttons. These tests instead throw random
// actions at a running simulator and check, after every step, the things that must always be true
// of a charging network no matter how it got there.

const (
	stressTimeout     = 30 * time.Second
	stressWorkerCount = 8
)

// The full runs are for CI; `go test -short` keeps the edit-test loop quick.
func randomWalkSteps() int {
	if testing.Short() {
		return 40
	}

	return 250
}

func stressActions() int {
	if testing.Short() {
		return 10
	}

	return 60
}

// seededRandomGateway makes the simulation's own dice reproducible, so a failing walk can be
// replayed from its seed. It is locked because requests and the tick roll it concurrently.
type seededRandomGateway struct {
	generator *rand.Rand
	mu        sync.Mutex
}

func newSeededRandomGateway(seed int64) *seededRandomGateway {
	return &seededRandomGateway{generator: rand.New(rand.NewSource(seed))}
}

func (g *seededRandomGateway) Float64() float64 {
	g.mu.Lock()
	defer g.mu.Unlock()

	return g.generator.Float64()
}

var randomBehaviors = []string{
	`[]`,
	`[{"kind": "realistic_reliability", "params": {"start_failure_rate": 0.3, "session_faults_per_hour": 2}}]`,
	`[{"kind": "reject_start"}]`,
	`[{"kind": "start_fails", "params": {"delay_s": 5}}]`,
	`[{"kind": "start_timeout", "params": {"timeout_s": 20}}]`,
	`[{"kind": "fault_mid_session", "params": {"after_s": 90}}]`,
}

// randomAction does one thing a person, a phone or an operator might do, and reports the HTTP
// status. It is weighted towards what people mostly do (plug in, start, stop), because a walk that
// rarely gets a car charging exercises very little. Many actions are still refused; that is the
// point. What must never happen is a 5xx.
func (f fixture) randomAction(t *testing.T, generator *rand.Rand, view worldView) (string, int) {
	roll := generator.Intn(100)

	// Mostly aim an action at a charger it makes sense for, the way a person would; sometimes at
	// any charger at all, the way a confused person or a buggy client would.
	preferredState := ""
	switch {
	case roll < 20:
		preferredState = "AVAILABLE"
	case roll < 44:
		preferredState = "PREPARING"
	case roll >= 47 && roll < 55:
		preferredState = "FINISHING"
	case roll >= 55 && roll < 62:
		preferredState = "CHARGING"
	case roll >= 75 && roll < 83:
		preferredState = "FAULTED"
	}

	chargerID, siteID := "EVSE-MISSING", validHubSiteID
	if picked, ok := pickCharger(generator, view, preferredState); ok {
		chargerID, siteID = picked.ChargerID, picked.SiteID
	}

	sessionID := "SES-MISSING"
	for _, session := range view.Sessions {
		if session.State == "ACTIVE" {
			sessionID = session.SessionID
		}
	}

	chargerPath := "/api/chargers/" + chargerID
	evseBody := `{"location_id": "` + siteID + `", "evse_uid": "` + chargerID + `"}`

	var method, path, body string
	switch {
	case roll < 20:
		method, path = http.MethodPost, chargerPath+"/actions/plug-in"
		body = fmt.Sprintf(`{"battery_capacity_kwh": 40, "max_power_kw": 120, "state_of_charge": %.2f}`, generator.Float64()*0.99)
	case roll < 44:
		method, path, body = http.MethodPost, "/emsp/api/start", evseBody
	case roll < 47:
		method, path, body = http.MethodPost, "/emsp/api/start", `{"location_id": "SITE-WRONG", "evse_uid": "`+chargerID+`"}`
	case roll < 55:
		method, path = http.MethodPost, chargerPath+"/actions/unplug"
	case roll < 62:
		method, path = http.MethodPost, chargerPath+"/actions/press-stop"
	case roll < 70:
		method, path, body = http.MethodPost, "/emsp/api/stop", `{"session_id": "`+sessionID+`"}`
	case roll < 75:
		method, path = http.MethodPost, chargerPath+"/actions/inject-fault"
	case roll < 83:
		method, path = http.MethodPost, chargerPath+"/actions/clear-fault"
	case roll < 87:
		method, path, body = http.MethodPost, "/emsp/api/unlock", evseBody
	case roll < 93:
		method, path, body = http.MethodPut, chargerPath+"/behaviors", randomBehaviors[generator.Intn(len(randomBehaviors))]
	case roll < 98:
		method, path, body = http.MethodPost, "/api/chargers", `{"site_id": "`+siteID+`"}`
	default:
		method, path = http.MethodDelete, chargerPath
	}

	statusCode, _ := f.request(t, method, path, body)

	return method + " " + path + " " + body, statusCode
}

func pickCharger(generator *rand.Rand, view worldView, preferredState string) (chargerView, bool) {
	if len(view.Chargers) == 0 {
		return chargerView{}, false
	}

	preferred := []chargerView{}
	for _, charger := range view.Chargers {
		if charger.State == preferredState {
			preferred = append(preferred, charger)
		}
	}

	if len(preferred) > 0 && generator.Intn(10) < 8 {
		return preferred[generator.Intn(len(preferred))], true
	}

	return view.Chargers[generator.Intn(len(view.Chargers))], true
}

// checkInvariants returns the first thing that is wrong with the world, or "".
func checkInvariants(view worldView, lastEnergyBySessionID map[string]float64) string {
	sessionBySessionID := map[string]sessionView{}
	completedCount := 0
	for _, session := range view.Sessions {
		sessionBySessionID[session.SessionID] = session

		if session.State == "COMPLETED" {
			completedCount++
		}

		if session.EnergyDeliveredKWH < lastEnergyBySessionID[session.SessionID] {
			return fmt.Sprintf("session %s lost energy: %v kWh, was %v kWh", session.SessionID, session.EnergyDeliveredKWH, lastEnergyBySessionID[session.SessionID])
		}

		lastEnergyBySessionID[session.SessionID] = session.EnergyDeliveredKWH
	}

	chargerCountBySessionID := map[string]int{}
	for _, charger := range view.Chargers {
		plugged := charger.Vehicle != nil

		switch charger.State {
		case "AVAILABLE":
			if plugged || charger.ConnectorLocked || charger.SessionID != "" {
				return fmt.Sprintf("charger %s is AVAILABLE but not idle: %+v", charger.ChargerID, charger)
			}
		case "PREPARING", "FINISHING":
			if !plugged || charger.ConnectorLocked || charger.SessionID != "" {
				return fmt.Sprintf("charger %s is %s but plugged=%v locked=%v session=%q", charger.ChargerID, charger.State, plugged, charger.ConnectorLocked, charger.SessionID)
			}
		case "CHARGING":
			if !plugged || !charger.ConnectorLocked || charger.SessionID == "" {
				return fmt.Sprintf("charger %s is CHARGING but plugged=%v locked=%v session=%q", charger.ChargerID, plugged, charger.ConnectorLocked, charger.SessionID)
			}
		case "FAULTED":
			if charger.SessionID != "" || (charger.ConnectorLocked && !plugged) {
				return fmt.Sprintf("charger %s is FAULTED but session=%q locked=%v plugged=%v", charger.ChargerID, charger.SessionID, charger.ConnectorLocked, plugged)
			}
		default:
			return fmt.Sprintf("charger %s is in unknown state %q", charger.ChargerID, charger.State)
		}

		if charger.LiveStateOfCharge != nil && (*charger.LiveStateOfCharge < 0 || *charger.LiveStateOfCharge > 1) {
			return fmt.Sprintf("charger %s reports state of charge %v", charger.ChargerID, *charger.LiveStateOfCharge)
		}

		if charger.SessionID == "" {
			continue
		}

		chargerCountBySessionID[charger.SessionID]++
		session, ok := sessionBySessionID[charger.SessionID]
		if !ok || session.State != "ACTIVE" || session.ChargerID != charger.ChargerID {
			return fmt.Sprintf("charger %s points at session %q, which is %+v", charger.ChargerID, charger.SessionID, session)
		}

		if session.EnergyDeliveredKWH > charger.Vehicle.BatteryCapacityKWH+1e-9 {
			return fmt.Sprintf("session %s delivered %v kWh into a %v kWh battery", session.SessionID, session.EnergyDeliveredKWH, charger.Vehicle.BatteryCapacityKWH)
		}
	}

	for _, session := range view.Sessions {
		if session.State == "ACTIVE" && chargerCountBySessionID[session.SessionID] != 1 {
			return fmt.Sprintf("active session %s is held by %d chargers", session.SessionID, chargerCountBySessionID[session.SessionID])
		}
	}

	if len(view.CDRs) != completedCount {
		return fmt.Sprintf("%d completed sessions but %d CDRs", completedCount, len(view.CDRs))
	}

	billedSessionIDs := map[string]bool{}
	for _, cdr := range view.CDRs {
		session := sessionBySessionID[cdr.SessionID]
		// the bill is the session's own cost, however that came to be computed
		if billedSessionIDs[cdr.SessionID] || session.State != "COMPLETED" || session.EnergyDeliveredKWH != cdr.EnergyDeliveredKWH || session.TotalCost != cdr.TotalCost {
			return fmt.Sprintf("CDR %s does not match its session: %+v vs %+v", cdr.CDRID, cdr, session)
		}

		billedSessionIDs[cdr.SessionID] = true
	}

	return ""
}

func TestInvariants(t *testing.T) {
	for _, seed := range []int64{1, 2, 3} {
		t.Run(fmt.Sprintf("hold after every step of a random walk with seed %d", seed), func(t *testing.T) {
			// Given
			f := newMockEMSPFixtureWithRandom(t, newSeededRandomGateway(seed))
			generator := rand.New(rand.NewSource(seed))
			lastEnergyBySessionID := map[string]float64{}
			history := []string{}

			view := f.world(t)

			for step := 0; step < randomWalkSteps(); step++ {
				// When
				action, statusCode := f.randomAction(t, generator, view)
				history = append(history, fmt.Sprintf("%d %s", statusCode, action))
				f.advance(t, time.Duration(generator.Intn(120))*time.Second)

				// Then
				view = f.world(t)
				violation := checkInvariants(view, lastEnergyBySessionID)
				if statusCode >= http.StatusInternalServerError {
					violation = fmt.Sprintf("the simulator answered %d", statusCode)
				}

				if violation != "" {
					recent := history[max(0, len(history)-12):]
					t.Fatalf("step %d: %s\nlast actions:\n  %s", step, violation, joinLines(recent))
				}
			}

			// a walk that never got a car charging would prove nothing
			t.Logf("seed %d: %d sessions, %d CDRs, %d chargers", seed, len(view.Sessions), len(view.CDRs), len(view.Chargers))
			if !testing.Short() {
				enoughSessions := len(view.Sessions) >= 8
				assert.Equal(t, enoughSessions, true)
			}
		})
	}
}

func joinLines(lines []string) string {
	joined := ""
	for i, line := range lines {
		if i > 0 {
			joined += "\n  "
		}

		joined += line
	}

	return joined
}

func TestConcurrency(t *testing.T) {
	t.Run("stays consistent and never deadlocks while many clients act during ticks", func(t *testing.T) {
		// Given
		f := newMockEMSPFixtureWithRandom(t, newSeededRandomGateway(7))
		serverErrors := make(chan string, stressWorkerCount*stressActions()+1)
		finished := make(chan struct{})

		// When
		go func() {
			defer close(finished)

			var workers sync.WaitGroup
			for worker := 0; worker < stressWorkerCount; worker++ {
				workers.Add(1)

				go func(seed int64) {
					defer workers.Done()

					generator := rand.New(rand.NewSource(seed))
					for i := 0; i < stressActions(); i++ {
						action, statusCode := f.randomAction(t, generator, f.world(t))
						if statusCode >= http.StatusInternalServerError {
							serverErrors <- fmt.Sprintf("%d %s", statusCode, action)
						}
					}
				}(int64(worker))
			}

			workersDone := make(chan struct{})
			go func() {
				workers.Wait()
				close(workersDone)
			}()

			// keep the simulation ticking underneath the workers for as long as they run
			for ticking := true; ticking; {
				select {
				case <-workersDone:
					ticking = false
				default:
					f.wallClock.Advance(15 * time.Second)
					if err := f.fakeTicker.Tick(); err != nil {
						serverErrors <- "tick: " + err.Error()
						ticking = false
					}
				}
			}

			<-workersDone
		}()

		// Then
		select {
		case <-finished:
		case <-time.After(stressTimeout):
			t.Fatalf("still running after %v: the simulator has probably deadlocked", stressTimeout)
		}

		close(serverErrors)
		for serverError := range serverErrors {
			t.Fatalf("the simulator failed under load: %s", serverError)
		}

		f.advance(t, time.Second)
		violation := checkInvariants(f.world(t), map[string]float64{})
		assert.Equal(t, violation, "")
	})
}
