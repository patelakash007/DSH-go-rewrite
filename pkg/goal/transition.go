package goal

//
//	active   → active/paused/blocked/complete
//	paused   → active/complete
//	blocked  → active/complete

func CanTransition(from, to Phase) bool {
	if !to.Valid() || !from.Valid() {
		return false
	}
	if from == PhaseComplete {
		return false
	}
	switch to {
	case PhaseActive:

		return from == PhaseActive || from == PhasePaused || from == PhaseBlocked
	case PhasePaused, PhaseBlocked:

		return from == PhaseActive
	case PhaseComplete:

		return from == PhaseActive || from == PhasePaused || from == PhaseBlocked
	default:
		return false
	}
}
