package command

import (
	"fmt"
	"sort"
	"sync"

	"cposim/entity"
)

type Repository interface {
	Delete(commandID string) error
	Get(commandID string) (entity.Command, error)
	List() ([]entity.Command, error)
	Upsert(command entity.Command) error
}

type inMemoryRepository struct {
	commandByCommandID map[string]entity.Command
	mu                 sync.RWMutex
}

func NewInMemoryRepository() Repository {
	return &inMemoryRepository{
		commandByCommandID: map[string]entity.Command{},
	}
}

func (r *inMemoryRepository) Delete(commandID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.commandByCommandID[commandID]; !ok {
		return fmt.Errorf("command %q not found", commandID)
	}

	delete(r.commandByCommandID, commandID)

	return nil
}

func (r *inMemoryRepository) Get(commandID string) (entity.Command, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	command, ok := r.commandByCommandID[commandID]
	if !ok {
		return entity.Command{}, fmt.Errorf("command %q not found", commandID)
	}

	return command, nil
}

// List returns entities ordered by ID, which for generated IDs is creation order.
func (r *inMemoryRepository) List() ([]entity.Command, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	commands := make([]entity.Command, 0, len(r.commandByCommandID))
	for _, command := range r.commandByCommandID {
		commands = append(commands, command)
	}

	sort.Slice(commands, func(i, j int) bool {
		return commands[i].CommandID < commands[j].CommandID
	})

	return commands, nil
}

func (r *inMemoryRepository) Upsert(command entity.Command) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if command.CommandID == "" {
		return fmt.Errorf("command has no ID")
	}

	r.commandByCommandID[command.CommandID] = command

	return nil
}
