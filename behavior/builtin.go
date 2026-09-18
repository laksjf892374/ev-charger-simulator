package behavior

import (
	"time"

	"cposim/entity"
)

const (
	KindFaultMidSession      = "fault_mid_session"
	KindRealisticReliability = "realistic_reliability"
	KindRejectStart          = "reject_start"
	KindStartFails           = "start_fails"
	KindStartTimeout         = "start_timeout"
)

func init() {
	Register(
		KindFaultMidSession,
		"faults part-way through every session",
		"The charger faults part-way through a session: the session ends early and the cable stays locked.",
		faultMidSession{AfterSeconds: 120},
	)
	Register(
		KindRealisticReliability,
		"is about as reliable as a real one",
		"Like a real public charger: a small share of remote starts fail, and now and then a session is cut short by a fault. Set both rates to 0 for a perfect charger.",
		realisticReliability{SessionFaultsPerHour: 0.02, StartFailureRate: 0.05},
	)
	Register(
		KindRejectStart,
		"refuses every start request",
		"Remote start is refused immediately: the eMSP gets CommandResponse REJECTED and no result follows.",
		rejectStart{},
	)
	Register(
		KindStartFails,
		"accepts start requests, then fails them",
		"Remote start is accepted, but the charger then reports FAILED and no session starts.",
		startFails{DelaySeconds: 10},
	)
	Register(
		KindStartTimeout,
		"never answers a start request",
		"Remote start is accepted, but the charger never responds: the eMSP gets TIMEOUT after a long wait.",
		startTimeout{TimeoutSeconds: 60},
	)
}

type faultMidSession struct {
	AfterSeconds float64 `json:"after_s"`
}

func (b faultMidSession) InterceptTick(tick *Tick) {
	if tick.ChargingDuration >= seconds(b.AfterSeconds) {
		tick.Fault = true
	}
}

type realisticReliability struct {
	SessionFaultsPerHour float64 `json:"session_faults_per_hour"`
	StartFailureRate     float64 `json:"start_failure_rate"`
}

func (b realisticReliability) InterceptStart(attempt *StartAttempt) {
	if attempt.Roll < b.StartFailureRate {
		attempt.ForcedResult = entity.CommandResultFailed
		attempt.ResultMessage = "the charger did not start (random failure, as real chargers sometimes do)"
	}
}

func (b realisticReliability) InterceptTick(tick *Tick) {
	if tick.Roll < b.SessionFaultsPerHour*tick.Elapsed.Hours() {
		tick.Fault = true
	}
}

type rejectStart struct{}

func (rejectStart) InterceptStart(attempt *StartAttempt) {
	attempt.Reject = true
	attempt.RejectMessage = "charger refused the request"
}

type startFails struct {
	DelaySeconds float64 `json:"delay_s"`
}

func (b startFails) InterceptStart(attempt *StartAttempt) {
	attempt.ForcedResult = entity.CommandResultFailed
	attempt.ResultDelay = seconds(b.DelaySeconds)
	attempt.ResultMessage = "the charger reported that it could not start"
}

type startTimeout struct {
	TimeoutSeconds float64 `json:"timeout_s"`
}

func (b startTimeout) InterceptStart(attempt *StartAttempt) {
	attempt.ForcedResult = entity.CommandResultTimeout
	attempt.ResultDelay = seconds(b.TimeoutSeconds)
	attempt.ResultMessage = "the charger never answered"
}

func seconds(value float64) time.Duration {
	return time.Duration(value * float64(time.Second))
}
