package charger_test

import (
	"testing"

	"cposim/assert"
	"cposim/entity"
	chargerrepo "cposim/repository/charger"
)

func TestDelete(t *testing.T) {
	t.Run("returns an error when the charger does not exist", func(t *testing.T) {
		// Given
		repository := chargerrepo.NewInMemoryRepository()

		// When
		err := repository.Delete("missing")

		// Then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), `charger "missing" not found`)
	})

	t.Run("removes the charger", func(t *testing.T) {
		// Given
		repository := chargerrepo.NewInMemoryRepository()
		assert.NoError(t, repository.Upsert(entity.Charger{ChargerID: "A"}))

		// When
		err := repository.Delete("A")

		// Then
		assert.NoError(t, err)
		_, getErr := repository.Get("A")
		assert.Error(t, getErr)
	})
}

func TestGet(t *testing.T) {
	t.Run("returns an error when the charger does not exist", func(t *testing.T) {
		// Given
		repository := chargerrepo.NewInMemoryRepository()

		// When
		charger, err := repository.Get("missing")

		// Then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), `charger "missing" not found`)
		assert.Equal(t, charger, entity.Charger{})
	})

	t.Run("returns the stored charger", func(t *testing.T) {
		// Given
		repository := chargerrepo.NewInMemoryRepository()
		assert.NoError(t, repository.Upsert(entity.Charger{ChargerID: "A"}))

		// When
		charger, err := repository.Get("A")

		// Then
		assert.NoError(t, err)
		assert.Equal(t, charger, entity.Charger{ChargerID: "A"})
	})
}

func TestList(t *testing.T) {
	t.Run("returns every charger ordered by ID", func(t *testing.T) {
		// Given
		repository := chargerrepo.NewInMemoryRepository()
		assert.NoError(t, repository.Upsert(entity.Charger{ChargerID: "B"}))
		assert.NoError(t, repository.Upsert(entity.Charger{ChargerID: "A"}))

		// When
		chargers, err := repository.List()

		// Then
		assert.NoError(t, err)
		assert.Equal(t, chargers, []entity.Charger{{ChargerID: "A"}, {ChargerID: "B"}})
	})
}

func TestUpsert(t *testing.T) {
	t.Run("returns an error when the charger has no ID", func(t *testing.T) {
		// Given
		repository := chargerrepo.NewInMemoryRepository()

		// When
		err := repository.Upsert(entity.Charger{})

		// Then
		assert.Error(t, err)
	})

	t.Run("replaces an existing charger with the same ID", func(t *testing.T) {
		// Given
		repository := chargerrepo.NewInMemoryRepository()
		assert.NoError(t, repository.Upsert(entity.Charger{ChargerID: "A"}))

		// When
		err := repository.Upsert(entity.Charger{ChargerID: "A"})

		// Then
		assert.NoError(t, err)
		chargers, listErr := repository.List()
		assert.NoError(t, listErr)
		chargerCount := len(chargers)
		assert.Equal(t, chargerCount, 1)
	})
}
