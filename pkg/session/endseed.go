//

//

package session

import (
	"github.com/JopenChen/dsh-go/pkg/brand"
)

//

func MarkEndSeed(sl *SessionLog, parentID brand.SessionID) (uint64, error) {
	return sl.Append(EndSeedData{ParentSession: parentID})
}

func SeedEndSeq(events []SessionEvent) uint64 {
	var last uint64
	for _, ev := range events {
		if ev.Type == EventEndSeed {
			last = ev.Seq
		}
	}
	return last
}

func IsAfterEndSeed(events []SessionEvent, seq uint64) bool {
	seed := SeedEndSeq(events)
	if seed == 0 {

		return true
	}
	return seq > seed
}

func EndSeedMarker(header *SessionHeader) uint64 {
	if header == nil {
		return 0
	}
	return header.SeedLength
}
