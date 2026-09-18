package cdr_test

import (
	"testing"

	"cposim/assert"
	"cposim/entity"
	cdrrepo "cposim/repository/cdr"
)

func TestDelete(t *testing.T) {
	t.Run("returns an error when the cdr does not exist", func(t *testing.T) {
		// Given
		repository := cdrrepo.NewInMemoryRepository()

		// When
		err := repository.Delete("missing")

		// Then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), `cdr "missing" not found`)
	})

	t.Run("removes the cdr", func(t *testing.T) {
		// Given
		repository := cdrrepo.NewInMemoryRepository()
		assert.NoError(t, repository.Upsert(entity.CDR{CDRID: "A"}))

		// When
		err := repository.Delete("A")

		// Then
		assert.NoError(t, err)
		_, getErr := repository.Get("A")
		assert.Error(t, getErr)
	})
}

func TestGet(t *testing.T) {
	t.Run("returns an error when the cdr does not exist", func(t *testing.T) {
		// Given
		repository := cdrrepo.NewInMemoryRepository()

		// When
		cdr, err := repository.Get("missing")

		// Then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), `cdr "missing" not found`)
		assert.Equal(t, cdr, entity.CDR{})
	})

	t.Run("returns the stored cdr", func(t *testing.T) {
		// Given
		repository := cdrrepo.NewInMemoryRepository()
		assert.NoError(t, repository.Upsert(entity.CDR{CDRID: "A"}))

		// When
		cdr, err := repository.Get("A")

		// Then
		assert.NoError(t, err)
		assert.Equal(t, cdr, entity.CDR{CDRID: "A"})
	})
}

func TestList(t *testing.T) {
	t.Run("returns every cdr ordered by ID", func(t *testing.T) {
		// Given
		repository := cdrrepo.NewInMemoryRepository()
		assert.NoError(t, repository.Upsert(entity.CDR{CDRID: "B"}))
		assert.NoError(t, repository.Upsert(entity.CDR{CDRID: "A"}))

		// When
		cdrs, err := repository.List()

		// Then
		assert.NoError(t, err)
		assert.Equal(t, cdrs, []entity.CDR{{CDRID: "A"}, {CDRID: "B"}})
	})
}

func TestUpsert(t *testing.T) {
	t.Run("returns an error when the cdr has no ID", func(t *testing.T) {
		// Given
		repository := cdrrepo.NewInMemoryRepository()

		// When
		err := repository.Upsert(entity.CDR{})

		// Then
		assert.Error(t, err)
	})

	t.Run("replaces an existing cdr with the same ID", func(t *testing.T) {
		// Given
		repository := cdrrepo.NewInMemoryRepository()
		assert.NoError(t, repository.Upsert(entity.CDR{CDRID: "A"}))

		// When
		err := repository.Upsert(entity.CDR{CDRID: "A"})

		// Then
		assert.NoError(t, err)
		cdrs, listErr := repository.List()
		assert.NoError(t, listErr)
		cdrCount := len(cdrs)
		assert.Equal(t, cdrCount, 1)
	})
}
