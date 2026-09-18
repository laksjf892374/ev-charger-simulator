package behavior

import (
	"time"

	"cposim/entity"
)

const (
	KindFaultMidSession = "fault_mid_session"
	KindRejectStart     = "reject_start"
	KindStartFails      = "start_fails"
	KindStartTimeout    = "start_timeout"
)

func init() {
	Register(
		KindFaultMidSession,
		"The charger faults part-way through a session: the session ends early and the cable stays locked.",
		faultMidSession{AfterSeconds: 120},
	)
	Register(
		KindRejectStart,
		"Remote start is refused immediately: the eMSP gets CommandResponse REJECTED and no result follows.",
		rejectStart{},
	)
	Register(
		KindStartFails,
		"Remote start is accepted, but the charger then reports FAILED and no session starts.",
		startFails{DelaySeconds: 10},
	)
	Register(
		KindStartTimeout,
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
}

type startTimeout struct {
	TimeoutSeconds float64 `json:"timeout_s"`
}

func (b startTimeout) InterceptStart(attempt *StartAttempt) {
	attempt.ForcedResult = entity.CommandResultTimeout
	attempt.ResultDelay = seconds(b.TimeoutSeconds)
}

func seconds(value float64) time.Duration {
	return time.Duration(value * float64(time.Second))
}
