package command_test

import (
	"testing"

	"cposim/assert"
	"cposim/entity"
	commandrepo "cposim/repository/command"
)

func TestDelete(t *testing.T) {
	t.Run("returns an error when the command does not exist", func(t *testing.T) {
		// Given
		repository := commandrepo.NewInMemoryRepository()

		// When
		err := repository.Delete("missing")

		// Then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), `command "missing" not found`)
	})

	t.Run("removes the command", func(t *testing.T) {
		// Given
		repository := commandrepo.NewInMemoryRepository()
		assert.NoError(t, repository.Upsert(entity.Command{CommandID: "A"}))

		// When
		err := repository.Delete("A")

		// Then
		assert.NoError(t, err)
		_, getErr := repository.Get("A")
		assert.Error(t, getErr)
	})
}

func TestGet(t *testing.T) {
	t.Run("returns an error when the command does not exist", func(t *testing.T) {
		// Given
		repository := commandrepo.NewInMemoryRepository()

		// When
		command, err := repository.Get("missing")

		// Then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), `command "missing" not found`)
		assert.Equal(t, command, entity.Command{})
	})

	t.Run("returns the stored command", func(t *testing.T) {
		// Given
		repository := commandrepo.NewInMemoryRepository()
		assert.NoError(t, repository.Upsert(entity.Command{CommandID: "A"}))

		// When
		command, err := repository.Get("A")

		// Then
		assert.NoError(t, err)
		assert.Equal(t, command, entity.Command{CommandID: "A"})
	})
}

func TestList(t *testing.T) {
	t.Run("returns every command ordered by ID", func(t *testing.T) {
		// Given
		repository := commandrepo.NewInMemoryRepository()
		assert.NoError(t, repository.Upsert(entity.Command{CommandID: "B"}))
		assert.NoError(t, repository.Upsert(entity.Command{CommandID: "A"}))

		// When
		commands, err := repository.List()

		// Then
		assert.NoError(t, err)
		assert.Equal(t, commands, []entity.Command{{CommandID: "A"}, {CommandID: "B"}})
	})
}

func TestUpsert(t *testing.T) {
	t.Run("returns an error when the command has no ID", func(t *testing.T) {
		// Given
		repository := commandrepo.NewInMemoryRepository()

		// When
		err := repository.Upsert(entity.Command{})

		// Then
		assert.Error(t, err)
	})

	t.Run("replaces an existing command with the same ID", func(t *testing.T) {
		// Given
		repository := commandrepo.NewInMemoryRepository()
		assert.NoError(t, repository.Upsert(entity.Command{CommandID: "A"}))

		// When
		err := repository.Upsert(entity.Command{CommandID: "A"})

		// Then
		assert.NoError(t, err)
		commands, listErr := repository.List()
		assert.NoError(t, listErr)
		commandCount := len(commands)
		assert.Equal(t, commandCount, 1)
	})
}
