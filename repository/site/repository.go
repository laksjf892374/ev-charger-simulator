package site

import (
	"fmt"
	"sort"
	"sync"

	"cposim/entity"
)

type Repository interface {
	Delete(siteID string) error
	Get(siteID string) (entity.Site, error)
	List() ([]entity.Site, error)
	Upsert(site entity.Site) error
}

type inMemoryRepository struct {
	siteBySiteID map[string]entity.Site
	mu           sync.RWMutex
}

func NewInMemoryRepository() Repository {
	return &inMemoryRepository{
		siteBySiteID: map[string]entity.Site{},
	}
}

func (r *inMemoryRepository) Delete(siteID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.siteBySiteID[siteID]; !ok {
		return fmt.Errorf("site %q not found", siteID)
	}

	delete(r.siteBySiteID, siteID)

	return nil
}

func (r *inMemoryRepository) Get(siteID string) (entity.Site, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	site, ok := r.siteBySiteID[siteID]
	if !ok {
		return entity.Site{}, fmt.Errorf("site %q not found", siteID)
	}

	return site, nil
}

// List returns entities ordered by ID, which for generated IDs is creation order.
func (r *inMemoryRepository) List() ([]entity.Site, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sites := make([]entity.Site, 0, len(r.siteBySiteID))
	for _, site := range r.siteBySiteID {
		sites = append(sites, site)
	}

	sort.Slice(sites, func(i, j int) bool {
		return sites[i].SiteID < sites[j].SiteID
	})

	return sites, nil
}

func (r *inMemoryRepository) Upsert(site entity.Site) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if site.SiteID == "" {
		return fmt.Errorf("site has no ID")
	}

	r.siteBySiteID[site.SiteID] = site

	return nil
}
