package cdr

import (
	"fmt"
	"sort"
	"sync"

	"cposim/entity"
)

type Repository interface {
	Delete(cdrID string) error
	Get(cdrID string) (entity.CDR, error)
	List() ([]entity.CDR, error)
	Upsert(cdr entity.CDR) error
}

type inMemoryRepository struct {
	cdrByCDRID map[string]entity.CDR
	mu         sync.RWMutex
}

func NewInMemoryRepository() Repository {
	return &inMemoryRepository{
		cdrByCDRID: map[string]entity.CDR{},
	}
}

func (r *inMemoryRepository) Delete(cdrID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.cdrByCDRID[cdrID]; !ok {
		return fmt.Errorf("cdr %q not found", cdrID)
	}

	delete(r.cdrByCDRID, cdrID)

	return nil
}

func (r *inMemoryRepository) Get(cdrID string) (entity.CDR, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cdr, ok := r.cdrByCDRID[cdrID]
	if !ok {
		return entity.CDR{}, fmt.Errorf("cdr %q not found", cdrID)
	}

	return cdr, nil
}

// List returns entities ordered by ID, which for generated IDs is creation order.
func (r *inMemoryRepository) List() ([]entity.CDR, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cdrs := make([]entity.CDR, 0, len(r.cdrByCDRID))
	for _, cdr := range r.cdrByCDRID {
		cdrs = append(cdrs, cdr)
	}

	sort.Slice(cdrs, func(i, j int) bool {
		return cdrs[i].CDRID < cdrs[j].CDRID
	})

	return cdrs, nil
}

func (r *inMemoryRepository) Upsert(cdr entity.CDR) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if cdr.CDRID == "" {
		return fmt.Errorf("cdr has no ID")
	}

	r.cdrByCDRID[cdr.CDRID] = cdr

	return nil
}
