package charger

import (
	"fmt"
	"sort"
	"sync"

	"cposim/entity"
)

type Repository interface {
	Delete(chargerID string) error
	Get(chargerID string) (entity.Charger, error)
	List() ([]entity.Charger, error)
	Upsert(charger entity.Charger) error
}

type inMemoryRepository struct {
	chargerByChargerID map[string]entity.Charger
	mu                 sync.RWMutex
}

func NewInMemoryRepository() Repository {
	return &inMemoryRepository{
		chargerByChargerID: map[string]entity.Charger{},
	}
}

func (r *inMemoryRepository) Delete(chargerID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.chargerByChargerID[chargerID]; !ok {
		return fmt.Errorf("charger %q not found", chargerID)
	}

	delete(r.chargerByChargerID, chargerID)

	return nil
}

func (r *inMemoryRepository) Get(chargerID string) (entity.Charger, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	charger, ok := r.chargerByChargerID[chargerID]
	if !ok {
		return entity.Charger{}, fmt.Errorf("charger %q not found", chargerID)
	}

	return charger, nil
}

// List returns entities ordered by ID, which for generated IDs is creation order.
func (r *inMemoryRepository) List() ([]entity.Charger, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	chargers := make([]entity.Charger, 0, len(r.chargerByChargerID))
	for _, charger := range r.chargerByChargerID {
		chargers = append(chargers, charger)
	}

	sort.Slice(chargers, func(i, j int) bool {
		return chargers[i].ChargerID < chargers[j].ChargerID
	})

	return chargers, nil
}

func (r *inMemoryRepository) Upsert(charger entity.Charger) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if charger.ChargerID == "" {
		return fmt.Errorf("charger has no ID")
	}

	r.chargerByChargerID[charger.ChargerID] = charger

	return nil
}
