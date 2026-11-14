package rename

// DefaultThreshold is the minimum Score for Detect to call a pair a rename.
// Chosen slightly above the best-F1 point of the labeled set (0.59) so that
// precision is favored: a missed rename still shows up as REMOVED_KEY, but a
// wrong one would mislead.
const DefaultThreshold = 0.60
