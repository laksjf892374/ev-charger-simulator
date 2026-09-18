package behavior_test

import (
	"encoding/json"
	"testing"
	"time"

	"cposim/controller/behavior"
	"cposim/entity"
	"cposim/internal/assert"
)

func TestBuild(t *testing.T) {
	t.Run("returns an error when the behavior kind is unknown", func(t *testing.T) {
		// Given
		specs := []entity.BehaviorSpec{{Kind: "does_not_exist"}}

		// When
		_, err := behavior.Build(specs)

		// Then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), `unknown behavior kind "does_not_exist"`)
	})

	t.Run("returns an error when the params are not valid for the kind", func(t *testing.T) {
		// Given
		specs := []entity.BehaviorSpec{{
			Kind:   behavior.KindStartFails,
			Params: json.RawMessage(`{"delay_s": "soon"}`),
		}}

		// When
		_, err := behavior.Build(specs)

		// Then
		assert.Error(t, err)
	})
}

func TestValidation(t *testing.T) {
	t.Run("returns an error when a delay is negative or absurdly long", func(t *testing.T) {
		// Given
		negative := []entity.BehaviorSpec{{Kind: behavior.KindStartFails, Params: json.RawMessage(`{"delay_s": -1}`)}}
		absurd := []entity.BehaviorSpec{{Kind: behavior.KindStartTimeout, Params: json.RawMessage(`{"timeout_s": 1e18}`)}}

		// When
		_, negativeErr := behavior.Build(negative)
		_, absurdErr := behavior.Build(absurd)

		// Then
		assert.Error(t, negativeErr)
		assert.Error(t, absurdErr)
		assert.Contains(t, absurdErr.Error(), "timeout_s must be between 0 and 86400")
	})

	t.Run("returns an error when a failure rate is not a probability", func(t *testing.T) {
		// Given
		specs := []entity.BehaviorSpec{{Kind: behavior.KindRealisticReliability, Params: json.RawMessage(`{"start_failure_rate": 1.5}`)}}

		// When
		_, err := behavior.Build(specs)

		// Then
		assert.Error(t, err)
	})

	t.Run("returns an error when a charger is given too many behaviors", func(t *testing.T) {
		// Given
		specs := make([]entity.BehaviorSpec, behavior.MaxBehaviorsPerCharger+1)
		for i := range specs {
			specs[i] = entity.BehaviorSpec{Kind: behavior.KindRejectStart}
		}

		// When
		_, err := behavior.Build(specs)

		// Then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "too many behaviors")
	})
}

func TestCatalog(t *testing.T) {
	t.Run("lists the built-in behaviors ordered by kind with their default params", func(t *testing.T) {
		// When
		infos := behavior.Catalog()

		// Then
		kinds := []string{}
		for _, info := range infos {
			kinds = append(kinds, info.Kind)
		}
		assert.Equal(t, kinds, []string{
			behavior.KindFaultMidSession,
			behavior.KindRealisticReliability,
			behavior.KindRejectStart,
			behavior.KindStartFails,
			behavior.KindStartTimeout,
		})
		assert.Equal(t, string(infos[0].DefaultParams), `{"after_s":120}`)
	})
}

func TestApplyStartInterceptors(t *testing.T) {
	t.Run("leaves the attempt untouched when the charger has no behaviors", func(t *testing.T) {
		// Given
		attempt := behavior.StartAttempt{ResultDelay: 2 * time.Second}

		// When
		err := behavior.ApplyStartInterceptors(nil, &attempt)

		// Then
		assert.NoError(t, err)
		assert.Equal(t, attempt, behavior.StartAttempt{ResultDelay: 2 * time.Second})
	})

	t.Run("rejects the start for reject_start", func(t *testing.T) {
		// Given
		attempt := behavior.StartAttempt{}
		specs := []entity.BehaviorSpec{{Kind: behavior.KindRejectStart}}

		// When
		err := behavior.ApplyStartInterceptors(specs, &attempt)

		// Then
		assert.NoError(t, err)
		assert.Equal(t, attempt.Reject, true)
	})

	t.Run("forces a FAILED result after the configured delay for start_fails", func(t *testing.T) {
		// Given
		attempt := behavior.StartAttempt{}
		specs := []entity.BehaviorSpec{{
			Kind:   behavior.KindStartFails,
			Params: json.RawMessage(`{"delay_s": 5}`),
		}}

		// When
		err := behavior.ApplyStartInterceptors(specs, &attempt)

		// Then
		assert.NoError(t, err)
		assert.Equal(t, attempt.ForcedResult, entity.CommandResultFailed)
		assert.Equal(t, attempt.ResultDelay, 5*time.Second)
		assert.NotEqual(t, attempt.ResultMessage, "")
	})

	t.Run("forces a TIMEOUT result after the default timeout for start_timeout", func(t *testing.T) {
		// Given
		attempt := behavior.StartAttempt{}
		specs := []entity.BehaviorSpec{{Kind: behavior.KindStartTimeout}}

		// When
		err := behavior.ApplyStartInterceptors(specs, &attempt)

		// Then
		assert.NoError(t, err)
		assert.Equal(t, attempt.ForcedResult, entity.CommandResultTimeout)
		assert.Equal(t, attempt.ResultDelay, 60*time.Second)
	})
}

func TestRealisticReliability(t *testing.T) {
	specs := []entity.BehaviorSpec{{
		Kind:   behavior.KindRealisticReliability,
		Params: json.RawMessage(`{"start_failure_rate": 0.05, "session_faults_per_hour": 0.5}`),
	}}

	t.Run("lets a start through when the roll is at or above the failure rate", func(t *testing.T) {
		// Given
		attempt := behavior.StartAttempt{Roll: 0.05}

		// When
		err := behavior.ApplyStartInterceptors(specs, &attempt)

		// Then
		assert.NoError(t, err)
		assert.Equal(t, attempt.ForcedResult, entity.CommandResult(""))
	})

	t.Run("fails a start when the roll is below the failure rate", func(t *testing.T) {
		// Given
		attempt := behavior.StartAttempt{Roll: 0.049}

		// When
		err := behavior.ApplyStartInterceptors(specs, &attempt)

		// Then
		assert.NoError(t, err)
		assert.Equal(t, attempt.ForcedResult, entity.CommandResultFailed)
		assert.Contains(t, attempt.ResultMessage, "random failure")
	})

	t.Run("scales the hourly fault rate by the time the tick covers", func(t *testing.T) {
		// Given
		// 0.5 faults per hour over a 6-minute tick is a 5% chance
		faultingTick := behavior.Tick{Elapsed: 6 * time.Minute, Roll: 0.049}
		healthyTick := behavior.Tick{Elapsed: 6 * time.Minute, Roll: 0.05}

		// When
		assert.NoError(t, behavior.ApplyTickInterceptors(specs, &faultingTick))
		assert.NoError(t, behavior.ApplyTickInterceptors(specs, &healthyTick))

		// Then
		assert.Equal(t, faultingTick.Fault, true)
		assert.Equal(t, healthyTick.Fault, false)
	})

	t.Run("never fails anything when both rates are zero", func(t *testing.T) {
		// Given
		perfect := []entity.BehaviorSpec{{
			Kind:   behavior.KindRealisticReliability,
			Params: json.RawMessage(`{"start_failure_rate": 0, "session_faults_per_hour": 0}`),
		}}
		attempt := behavior.StartAttempt{Roll: 0}
		tick := behavior.Tick{Elapsed: time.Hour, Roll: 0}

		// When
		assert.NoError(t, behavior.ApplyStartInterceptors(perfect, &attempt))
		assert.NoError(t, behavior.ApplyTickInterceptors(perfect, &tick))

		// Then
		assert.Equal(t, attempt.ForcedResult, entity.CommandResult(""))
		assert.Equal(t, tick.Fault, false)
	})
}

func TestPrecedence(t *testing.T) {
	startFails := entity.BehaviorSpec{Kind: behavior.KindStartFails, Params: json.RawMessage(`{"delay_s": 5}`)}
	startTimeout := entity.BehaviorSpec{Kind: behavior.KindStartTimeout, Params: json.RawMessage(`{"timeout_s": 40}`)}
	rejectStart := entity.BehaviorSpec{Kind: behavior.KindRejectStart}

	t.Run("lets the later of two forced results win, delay and message included", func(t *testing.T) {
		// Given
		failsThenTimesOut := behavior.StartAttempt{}
		timesOutThenFails := behavior.StartAttempt{}

		// When
		assert.NoError(t, behavior.ApplyStartInterceptors([]entity.BehaviorSpec{startFails, startTimeout}, &failsThenTimesOut))
		assert.NoError(t, behavior.ApplyStartInterceptors([]entity.BehaviorSpec{startTimeout, startFails}, &timesOutThenFails))

		// Then
		assert.Equal(t, failsThenTimesOut.ForcedResult, entity.CommandResultTimeout)
		assert.Equal(t, failsThenTimesOut.ResultDelay, 40*time.Second)
		assert.Equal(t, failsThenTimesOut.ResultMessage, "the charger never answered")
		assert.Equal(t, timesOutThenFails.ForcedResult, entity.CommandResultFailed)
		assert.Equal(t, timesOutThenFails.ResultDelay, 5*time.Second)
	})

	t.Run("keeps a refusal final wherever it is in the list", func(t *testing.T) {
		// Given
		rejectedFirst := behavior.StartAttempt{}
		rejectedLast := behavior.StartAttempt{}

		// When
		assert.NoError(t, behavior.ApplyStartInterceptors([]entity.BehaviorSpec{rejectStart, startFails}, &rejectedFirst))
		assert.NoError(t, behavior.ApplyStartInterceptors([]entity.BehaviorSpec{startFails, rejectStart}, &rejectedLast))

		// Then
		assert.Equal(t, rejectedFirst.Reject, true)
		assert.Equal(t, rejectedLast.Reject, true)
	})

	t.Run("keeps a fault final even when a later behavior would not have faulted", func(t *testing.T) {
		// Given
		tick := behavior.Tick{ChargingDuration: 10 * time.Minute, Elapsed: time.Second, Roll: 0.99}
		specs := []entity.BehaviorSpec{
			{Kind: behavior.KindFaultMidSession},
			{Kind: behavior.KindRealisticReliability},
		}

		// When
		err := behavior.ApplyTickInterceptors(specs, &tick)

		// Then
		assert.NoError(t, err)
		assert.Equal(t, tick.Fault, true)
	})
}

func TestApplyTickInterceptors(t *testing.T) {
	t.Run("does not fault before the configured charging duration", func(t *testing.T) {
		// Given
		tick := behavior.Tick{ChargingDuration: 119 * time.Second}
		specs := []entity.BehaviorSpec{{Kind: behavior.KindFaultMidSession}}

		// When
		err := behavior.ApplyTickInterceptors(specs, &tick)

		// Then
		assert.NoError(t, err)
		assert.Equal(t, tick.Fault, false)
	})

	t.Run("faults once the configured charging duration has passed", func(t *testing.T) {
		// Given
		tick := behavior.Tick{ChargingDuration: 120 * time.Second}
		specs := []entity.BehaviorSpec{{Kind: behavior.KindFaultMidSession}}

		// When
		err := behavior.ApplyTickInterceptors(specs, &tick)

		// Then
		assert.NoError(t, err)
		assert.Equal(t, tick.Fault, true)
	})
}
