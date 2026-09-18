package identifier

import (
	"fmt"
	"sync"
)

type Gateway interface {
	NewID(prefix string) string
}

type sequentialGateway struct {
	lastNumberByPrefix map[string]int
	mu                 sync.Mutex
}

// NewSequentialGateway issues IDs like SES-000001. They are readable in a demo and sort in
// creation order.
func NewSequentialGateway() Gateway {
	return &sequentialGateway{
		lastNumberByPrefix: map[string]int{},
	}
}

func (g *sequentialGateway) NewID(prefix string) string {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.lastNumberByPrefix[prefix]++

	return fmt.Sprintf("%s-%06d", prefix, g.lastNumberByPrefix[prefix])
}
