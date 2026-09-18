// Package mockemsp is a deliberately small eMSP: the backend of a driver's charging app. It exists
// so the simulator can be demonstrated without a real eMSP, and it sits at the edge of the
// system: it talks to the CPO only over HTTP, exactly like a real eMSP would, and nothing in the
// simulator imports it except the DI root.
//
// It is naive on purpose. It believes whatever the CPO pushes, in the order it arrives, which
// makes the consequences of a misbehaving CPO easy to see.
package mockemsp

import (
	"sort"
	"sync"

	"cposim/ocpi"
)

const (
	CommandResultPending = "PENDING"

	evseStatusRemoved = "REMOVED"
)

// Command is a request the driver's app made, and what came of it.
type Command struct {
	EVSEUID string `json:"evse_uid,omitempty"`
	Kind    string `json:"kind"`
	Message string `json:"message,omitempty"`
	// The CPO's immediate answer: ACCEPTED, REJECTED, …
	Response string `json:"response"`
	// The outcome the CPO reports later; PENDING until it arrives (if it ever does).
	Result    string `json:"result"`
	SessionID string `json:"session_id,omitempty"`
	UID       string `json:"uid"`
}

// State is everything the eMSP believes about the world.
type State struct {
	CDRs      []ocpi.CDR      `json:"cdrs"`
	Commands  []Command       `json:"commands"`
	Locations []ocpi.Location `json:"locations"`
	Sessions  []ocpi.Session  `json:"sessions"`
}

type store struct {
	cdrs                 []ocpi.CDR
	commands             []Command
	locationByLocationID map[string]ocpi.Location
	sessionBySessionID   map[string]ocpi.Session
	mu                   sync.Mutex
}

func newStore() *store {
	return &store{
		locationByLocationID: map[string]ocpi.Location{},
		sessionBySessionID:   map[string]ocpi.Session{},
	}
}

func (s *store) state() State {
	s.mu.Lock()
	defer s.mu.Unlock()

	state := State{
		CDRs:      append([]ocpi.CDR{}, s.cdrs...),
		Commands:  append([]Command{}, s.commands...),
		Locations: []ocpi.Location{},
		Sessions:  []ocpi.Session{},
	}

	for _, location := range s.locationByLocationID {
		state.Locations = append(state.Locations, location)
	}

	for _, session := range s.sessionBySessionID {
		state.Sessions = append(state.Sessions, session)
	}

	sort.Slice(state.Locations, func(i, j int) bool { return state.Locations[i].ID < state.Locations[j].ID })
	sort.Slice(state.Sessions, func(i, j int) bool { return state.Sessions[i].ID < state.Sessions[j].ID })

	return state
}

// putLocation replaces a location. EVSEs already known are kept when the new object has none,
// because a CPO may announce a location before (or without) its EVSEs.
func (s *store) putLocation(location ocpi.Location) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(location.EVSEs) == 0 {
		location.EVSEs = s.locationByLocationID[location.ID].EVSEs
	}

	s.locationByLocationID[location.ID] = location
}

func (s *store) putEVSE(locationID string, evse ocpi.EVSE) {
	s.mu.Lock()
	defer s.mu.Unlock()

	location := s.locationByLocationID[locationID]
	location.ID = locationID

	evses := []ocpi.EVSE{}
	for _, existing := range location.EVSEs {
		if existing.UID != evse.UID {
			evses = append(evses, existing)
		}
	}

	if evse.Status != evseStatusRemoved {
		evses = append(evses, evse)
	}

	sort.Slice(evses, func(i, j int) bool { return evses[i].UID < evses[j].UID })
	location.EVSEs = evses
	s.locationByLocationID[locationID] = location
}

func (s *store) putSession(session ocpi.Session) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sessionBySessionID[session.ID] = session
}

// addCDR appends without checking for duplicates: a CDR is a bill, and this eMSP bills whatever
// it is sent.
func (s *store) addCDR(cdr ocpi.CDR) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cdrs = append(s.cdrs, cdr)
}

func (s *store) addCommand(command Command) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.commands = append(s.commands, command)
}

func (s *store) resolveCommand(uid string, result ocpi.CommandResult) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.commands {
		if s.commands[i].UID != uid {
			continue
		}

		s.commands[i].Result = result.Result
		if len(result.Message) > 0 {
			s.commands[i].Message = result.Message[0].Text
		}

		return true
	}

	return false
}

// answerCommand records the CPO's synchronous answer. Anything other than ACCEPTED is final: no
// result will follow, so the answer doubles as the outcome.
func (s *store) answerCommand(uid string, response string, message string) Command {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.commands {
		if s.commands[i].UID != uid {
			continue
		}

		s.commands[i].Response = response
		if message != "" {
			s.commands[i].Message = message
		}

		if response != ocpi.CommandResponseAccepted && s.commands[i].Result == CommandResultPending {
			s.commands[i].Result = response
		}

		return s.commands[i]
	}

	return Command{}
}
