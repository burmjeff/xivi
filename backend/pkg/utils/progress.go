package utils

// ProgressReporter receives a best-effort completion percentage and a
// user-facing description of the current phase. A progress value of zero means
// the phase is active but does not yet have a measurable total.
type ProgressReporter func(progress int, message string)

func reportProgress(reporter ProgressReporter, progress int, message string) {
	if reporter != nil {
		reporter(progress, message)
	}
}
