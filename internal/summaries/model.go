package summaries

import "ballast/internal/modelfetch"

// Model is the summary model, chosen by testing candidates on real videos
// (research.md §2, tasks T004) and pinned here by revision URL, size and
// SHA-256 once the maintainer approves. Until then it is unset, and
// Availability reports ReasonModelNotChosen.
var Model = modelfetch.Spec{}
