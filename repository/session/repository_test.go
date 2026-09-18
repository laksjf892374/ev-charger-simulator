package session_test

import (
	"testing"

	"cposim/entity"
	"cposim/internal/assert"
	sessionrepo "cposim/repository/session"
)

func TestDelete(t *testing.T) {
	t.Run("returns an error when the session does not exist", func(t *testing.T) {
		// Given
		repository := sessionrepo.NewInMemoryRepository()

		// When
		err := repository.Delete("missing")

		// Then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), `session "missing" not found`)
	})

	t.Run("removes the session", func(t *testing.T) {
		// Given
		repository := sessionrepo.NewInMemoryRepository()
		assert.NoError(t, repository.Upsert(entity.Session{SessionID: "A"}))

		// When
		err := repository.Delete("A")

		// Then
		assert.NoError(t, err)
		_, getErr := repository.Get("A")
		assert.Error(t, getErr)
	})
}

func TestGet(t *testing.T) {
	t.Run("returns an error when the session does not exist", func(t *testing.T) {
		// Given
		repository := sessionrepo.NewInMemoryRepository()

		// When
		session, err := repository.Get("missing")

		// Then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), `session "missing" not found`)
		assert.Equal(t, session, entity.Session{})
	})

	t.Run("returns the stored session", func(t *testing.T) {
		// Given
		repository := sessionrepo.NewInMemoryRepository()
		assert.NoError(t, repository.Upsert(entity.Session{SessionID: "A"}))

		// When
		session, err := repository.Get("A")

		// Then
		assert.NoError(t, err)
		assert.Equal(t, session, entity.Session{SessionID: "A"})
	})
}

func TestList(t *testing.T) {
	t.Run("returns every session ordered by ID", func(t *testing.T) {
		// Given
		repository := sessionrepo.NewInMemoryRepository()
		assert.NoError(t, repository.Upsert(entity.Session{SessionID: "B"}))
		assert.NoError(t, repository.Upsert(entity.Session{SessionID: "A"}))

		// When
		sessions, err := repository.List()

		// Then
		assert.NoError(t, err)
		assert.Equal(t, sessions, []entity.Session{{SessionID: "A"}, {SessionID: "B"}})
	})
}

func TestUpsert(t *testing.T) {
	t.Run("returns an error when the session has no ID", func(t *testing.T) {
		// Given
		repository := sessionrepo.NewInMemoryRepository()

		// When
		err := repository.Upsert(entity.Session{})

		// Then
		assert.Error(t, err)
	})

	t.Run("replaces an existing session with the same ID", func(t *testing.T) {
		// Given
		repository := sessionrepo.NewInMemoryRepository()
		assert.NoError(t, repository.Upsert(entity.Session{SessionID: "A"}))

		// When
		err := repository.Upsert(entity.Session{SessionID: "A"})

		// Then
		assert.NoError(t, err)
		sessions, listErr := repository.List()
		assert.NoError(t, listErr)
		sessionCount := len(sessions)
		assert.Equal(t, sessionCount, 1)
	})
}
