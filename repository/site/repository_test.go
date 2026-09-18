package site_test

import (
	"testing"

	"cposim/assert"
	"cposim/entity"
	siterepo "cposim/repository/site"
)

func TestDelete(t *testing.T) {
	t.Run("returns an error when the site does not exist", func(t *testing.T) {
		// Given
		repository := siterepo.NewInMemoryRepository()

		// When
		err := repository.Delete("missing")

		// Then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), `site "missing" not found`)
	})

	t.Run("removes the site", func(t *testing.T) {
		// Given
		repository := siterepo.NewInMemoryRepository()
		assert.NoError(t, repository.Upsert(entity.Site{SiteID: "A"}))

		// When
		err := repository.Delete("A")

		// Then
		assert.NoError(t, err)
		_, getErr := repository.Get("A")
		assert.Error(t, getErr)
	})
}

func TestGet(t *testing.T) {
	t.Run("returns an error when the site does not exist", func(t *testing.T) {
		// Given
		repository := siterepo.NewInMemoryRepository()

		// When
		site, err := repository.Get("missing")

		// Then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), `site "missing" not found`)
		assert.Equal(t, site, entity.Site{})
	})

	t.Run("returns the stored site", func(t *testing.T) {
		// Given
		repository := siterepo.NewInMemoryRepository()
		assert.NoError(t, repository.Upsert(entity.Site{SiteID: "A"}))

		// When
		site, err := repository.Get("A")

		// Then
		assert.NoError(t, err)
		assert.Equal(t, site, entity.Site{SiteID: "A"})
	})
}

func TestList(t *testing.T) {
	t.Run("returns every site ordered by ID", func(t *testing.T) {
		// Given
		repository := siterepo.NewInMemoryRepository()
		assert.NoError(t, repository.Upsert(entity.Site{SiteID: "B"}))
		assert.NoError(t, repository.Upsert(entity.Site{SiteID: "A"}))

		// When
		sites, err := repository.List()

		// Then
		assert.NoError(t, err)
		assert.Equal(t, sites, []entity.Site{{SiteID: "A"}, {SiteID: "B"}})
	})
}

func TestUpsert(t *testing.T) {
	t.Run("returns an error when the site has no ID", func(t *testing.T) {
		// Given
		repository := siterepo.NewInMemoryRepository()

		// When
		err := repository.Upsert(entity.Site{})

		// Then
		assert.Error(t, err)
	})

	t.Run("replaces an existing site with the same ID", func(t *testing.T) {
		// Given
		repository := siterepo.NewInMemoryRepository()
		assert.NoError(t, repository.Upsert(entity.Site{SiteID: "A"}))

		// When
		err := repository.Upsert(entity.Site{SiteID: "A"})

		// Then
		assert.NoError(t, err)
		sites, listErr := repository.List()
		assert.NoError(t, listErr)
		siteCount := len(sites)
		assert.Equal(t, siteCount, 1)
	})
}
