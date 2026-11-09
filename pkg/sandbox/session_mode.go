//

//

package sandbox

import (
	"github.com/JopenChen/dsh-go/pkg/session"
)

type ModeSource string

const (
	ModeSourceDelegation ModeSource = "delegation"
)

var SandboxModes = []SandboxMode{
	ModeReadOnly,
	ModeWorkspaceWrite,
	ModeDangerFullAccess,
}

func EffectiveSandboxMode(events []session.SessionEvent) (SandboxMode, bool) {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type != session.EventSandboxMode {
			continue
		}
		data, ok := events[i].Data.(session.SandboxModeData)
		if !ok {
			continue
		}
		return SandboxMode(data.Mode), true
	}
	return "", false
}

type SessionAppender interface {
	Append(data session.EventData) (uint64, error)
}

func SetSandboxMode(sess SessionAppender, mode SandboxMode) error {
	_, err := sess.Append(session.SandboxModeData{Mode: string(mode)})
	return err
}

func SetSandboxModeWithSource(sess SessionAppender, mode SandboxMode, source ModeSource) error {
	_, err := sess.Append(session.SandboxModeData{Mode: string(mode), Source: string(source)})
	return err
}
