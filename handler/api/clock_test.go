package api_test

import (
	"errors"
	"net/http"
	"testing"

	"cposim/internal/assert"
)

func TestUpdateClock(t *testing.T) {
	t.Run("reports why the clock refused the speed", func(t *testing.T) {
		// Given
		f := newFixture(t)
		f.clockGateway.SetSpeedErr = errors.New("speed must be positive")

		// When
		recorder := serve(t, f, http.MethodPut, "/api/clock", `{"speed": 0}`)

		// Then
		assert.Equal(t, recorder.Code, http.StatusUnprocessableEntity)
	})

	t.Run("sets the simulation speed", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		recorder := serve(t, f, http.MethodPut, "/api/clock", `{"speed": 60}`)

		// Then
		assert.Equal(t, recorder.Code, http.StatusOK)
		assert.Equal(t, f.clockGateway.SetSpeedCalledWith, []float64{60})
	})
}
