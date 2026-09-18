// Package behavior makes chargers deviate from the happy path.
//
// A behavior is a small stateless type whose JSON form is its parameter set. It takes part in
// the simulation by implementing one or more of the interceptor interfaces. To add a scenario:
// write the type, implement the hooks it needs, and add one Register call in builtin.go. The
// control API and UI pick it up from Catalog without changes.
//
// # When a charger has more than one behavior
//
// Behaviors are applied in the order the charger lists them, each seeing what the earlier ones
// decided. Two rules settle disagreements, and both are pinned by tests:
//
//   - A refusal or a fault is final. Once any behavior sets Reject (on a start) or Fault (on a
//     tick), no later behavior can take it back, and a rejected start ignores any forced result.
//   - Everything else is last-writer-wins. A later behavior's ForcedResult, ResultDelay and
//     ResultMessage replace an earlier one's. PowerFactor multiplies, so it composes instead.
//
// All behaviors of one charger share a single random Roll per event. Two probabilistic behaviors
// on the same charger are therefore correlated, not independent; give a behavior its own roll
// (a new field on the context struct, drawn by the controller) if that ever matters.
package behavior

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"cposim/entity"
)

const MaxBehaviorsPerCharger = 8

type Behavior any

// StartInterceptor can change how a remote start plays out.
type StartInterceptor interface {
	InterceptStart(attempt *StartAttempt)
}

// Validator is implemented by behaviors whose params can be out of range. Build refuses a spec
// that fails it, so a charger can never be configured into nonsense.
type Validator interface {
	Validate() error
}

// TickInterceptor is consulted on every simulation tick of a charging session.
type TickInterceptor interface {
	InterceptTick(tick *Tick)
}

type StartAttempt struct {
	Charger entity.Charger
	// When set, this result is reported after ResultDelay instead of trying to start.
	ForcedResult  entity.CommandResult
	Reject        bool
	RejectMessage string
	ResultDelay   time.Duration
	ResultMessage string
	// A random number in [0, 1), drawn once per attempt, for probabilistic behaviors. Behaviors
	// never draw their own, so the caller decides where randomness comes from.
	Roll float64
}

type Tick struct {
	Charger          entity.Charger
	ChargingDuration time.Duration
	// Simulated time this tick covers; a per-hour rate becomes a per-tick probability through it.
	Elapsed     time.Duration
	Fault       bool
	PowerFactor float64
	// A random number in [0, 1), drawn once per charger per tick.
	Roll    float64
	Session entity.Session
}

type Info struct {
	DefaultParams json.RawMessage `json:"default_params,omitempty"`
	Description   string          `json:"description"`
	Kind          string          `json:"kind"`
	// A few words that complete the sentence "This charger …", for display next to a charger.
	Label string `json:"label"`
}

type registration struct {
	build func(params json.RawMessage) (Behavior, error)
	info  Info
}

var registrationByKind = map[string]registration{}

// Register makes a behavior kind available. defaults supplies the value of every parameter a
// spec leaves out.
func Register[T Behavior](kind string, label string, description string, defaults T) {
	defaultParams, _ := json.Marshal(defaults)
	if string(defaultParams) == "{}" {
		defaultParams = nil
	}

	registrationByKind[kind] = registration{
		build: func(params json.RawMessage) (Behavior, error) {
			built := defaults
			if len(params) == 0 {
				return built, nil
			}

			if err := json.Unmarshal(params, &built); err != nil {
				return nil, fmt.Errorf("json.Unmarshal: %w", err)
			}

			return built, nil
		},
		info: Info{
			DefaultParams: defaultParams,
			Description:   description,
			Kind:          kind,
			Label:         label,
		},
	}
}

func Catalog() []Info {
	infos := make([]Info, 0, len(registrationByKind))
	for _, registered := range registrationByKind {
		infos = append(infos, registered.info)
	}

	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Kind < infos[j].Kind
	})

	return infos
}

func Build(specs []entity.BehaviorSpec) ([]Behavior, error) {
	if len(specs) > MaxBehaviorsPerCharger {
		return nil, fmt.Errorf("too many behaviors: %d, limit %d", len(specs), MaxBehaviorsPerCharger)
	}

	behaviors := make([]Behavior, 0, len(specs))
	for _, spec := range specs {
		registered, ok := registrationByKind[spec.Kind]
		if !ok {
			return nil, fmt.Errorf("unknown behavior kind %q", spec.Kind)
		}

		built, err := registered.build(spec.Params)
		if err != nil {
			return nil, fmt.Errorf("registered.build: %w", err)
		}

		if validator, ok := built.(Validator); ok {
			if err := validator.Validate(); err != nil {
				return nil, fmt.Errorf("validator.Validate: %w", err)
			}
		}

		behaviors = append(behaviors, built)
	}

	return behaviors, nil
}

func ApplyStartInterceptors(specs []entity.BehaviorSpec, attempt *StartAttempt) error {
	behaviors, err := Build(specs)
	if err != nil {
		return fmt.Errorf("Build: %w", err)
	}

	for _, built := range behaviors {
		if interceptor, ok := built.(StartInterceptor); ok {
			interceptor.InterceptStart(attempt)
		}
	}

	return nil
}

func ApplyTickInterceptors(specs []entity.BehaviorSpec, tick *Tick) error {
	behaviors, err := Build(specs)
	if err != nil {
		return fmt.Errorf("Build: %w", err)
	}

	for _, built := range behaviors {
		if interceptor, ok := built.(TickInterceptor); ok {
			interceptor.InterceptTick(tick)
		}
	}

	return nil
}
