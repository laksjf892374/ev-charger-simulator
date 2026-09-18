package behavior_test

import (
	"encoding/json"
	"testing"
	"time"

	"cposim/assert"
	"cposim/behavior"
	"cposim/entity"
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
