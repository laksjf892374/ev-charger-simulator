package charger_test

import (
	"testing"

	"cposim/entity"
	"cposim/internal/assert"
)

func TestAddSite(t *testing.T) {
	t.Run("returns an error when the site ID would not survive a URL path", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		_, err := f.chargerController.AddSite(entity.Site{Name: "Bad", SiteID: "../etc"})

		// Then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "site ID must be 1-36 letters")
	})

	t.Run("returns an error when the coordinates are out of range", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		_, err := f.chargerController.AddSite(entity.Site{Latitude: 91, Name: "Nowhere"})

		// Then
		assert.Error(t, err)
	})

	t.Run("returns an error when the site limit is reached", func(t *testing.T) {
		// Given
		f := newFixture(t)
		_, err := f.chargerController.AddSite(entity.Site{Name: "Second"})
		assert.NoError(t, err)

		// When
		_, err = f.chargerController.AddSite(entity.Site{Name: "Third"})

		// Then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "site limit reached: limit 2")
	})
}
