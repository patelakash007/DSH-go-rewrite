//

//

package session

import (
	"encoding/json"
	"fmt"
)

var KNOWN_SESSION_EVENT_TYPES = func() map[EventType]struct{} {
	m := map[EventType]struct{}{}
	for _, t := range AllEventTypes {
		m[t] = struct{}{}
	}
	return m
}()

type CorruptEventError struct {
	Seq  uint64
	Type EventType
}

func (e *CorruptEventError) Error() string {
	return fmt.Sprintf("session: corrupt event seq %d with unknown type %q", e.Seq, e.Type)
}

func IsCorruptEventError(err error) bool {
	_, ok := err.(*CorruptEventError)
	return ok
}

//

func LoadSession(headerBytes []byte, eventLines ...[]byte) (*SessionHeader, []SessionEvent, error) {
	header, err := UnmarshalSessionHeader(headerBytes)
	if err != nil {
		return nil, nil, err
	}
	if err := header.Validate(); err != nil {
		return nil, nil, err
	}

	events := make([]SessionEvent, 0, len(eventLines))
	for _, line := range eventLines {

		var probe struct {
			Seq  uint64    `json:"seq"`
			Type EventType `json:"type"`
		}
		if err := json.Unmarshal(line, &probe); err != nil {
			return nil, nil, err
		}
		if _, known := KNOWN_SESSION_EVENT_TYPES[probe.Type]; !known {
			return nil, nil, &CorruptEventError{Seq: probe.Seq, Type: probe.Type}
		}

		var ev SessionEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			return nil, nil, err
		}
		events = append(events, ev)
	}
	return header, events, nil
}
