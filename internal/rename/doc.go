// Package rename detects likely field renames: a removed key and an added key
// at the same path that are probably the same field under a new name.
//
// It scores each (removed, added) pair from type-level features only (names,
// kinds, sibling position, child shape, observation fraction). It never looks
// at values. See Detect for pairing and Ablation for how quality is measured.
package rename
