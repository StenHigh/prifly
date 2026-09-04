package local

import (
	"encoding/json"

	"github.com/stenhigh/prifly/internal/purity"
)

// These are the only places a command transform is entered. A transform
// decides from what it was given; the marked region lets a guarded operation
// report a transform that went looking for facts instead.
func applyTransform[S any](transform func(S) (Change, error), snapshot S) (Change, error) {
	defer purity.Enter()()
	return transform(snapshot)
}

func applyAuthorityTransform(transform func(AuthoritySnapshot) (AuthorityChange, error), snapshot AuthoritySnapshot) (AuthorityChange, error) {
	defer purity.Enter()()
	return transform(snapshot)
}

func applyControlMutation(mutation func(AuthoritySnapshot) (json.RawMessage, error), snapshot AuthoritySnapshot) (json.RawMessage, error) {
	defer purity.Enter()()
	return mutation(snapshot)
}
