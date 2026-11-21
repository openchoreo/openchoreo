// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package revision

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// act builds revision n, last activated at binding generation gen. previous
// is the revision it replaced then; -1 leaves AnnotationKeyPreviousRevision
// off, as on a revision recorded before it existed.
func act(n, gen, previous int64) Activation {
	annotations := map[string]string{AnnotationKeyBindingGeneration: strconv.FormatInt(gen, 10)}
	if previous >= 0 {
		annotations[AnnotationKeyPreviousRevision] = strconv.FormatInt(previous, 10)
	}
	return Activation{
		Object: &openchoreov1alpha1.ReleaseBindingRevision{ObjectMeta: metav1.ObjectMeta{
			Name: "b-r" + strconv.FormatInt(n, 10), Annotations: annotations}},
		Number: n,
	}
}

func TestPreviousActive(t *testing.T) {
	tests := []struct {
		name        string
		revs        []Activation
		highest     int64
		wantCurrent int64
		wantPrev    int64
		// wantInferred is true when the target is inferred from activation order.
		wantInferred bool
		wantErr      error
	}{
		{
			name:        "a normal sequence: the revision before the current one",
			revs:        []Activation{act(1, 1, 0), act(2, 2, 1), act(3, 3, 2)},
			highest:     3,
			wantCurrent: 3, wantPrev: 2,
		},
		{
			name: "after a rollback to r1, the previous revision is r3, numbered above the current one",
			// r1 -> r2 -> r3, then a rollback made r1 active again at generation 4.
			revs:        []Activation{act(1, 4, 3), act(2, 2, 1), act(3, 3, 2)},
			highest:     3,
			wantCurrent: 1, wantPrev: 3,
		},
		{
			name: "a second rollback follows activation history, not numbers",
			// r1 -> r2 -> r3 -> (rollback) r1 -> (rollback) r2.
			revs:        []Activation{act(1, 4, 3), act(2, 5, 1), act(3, 3, 2)},
			highest:     3,
			wantCurrent: 2, wantPrev: 1,
		},
		{
			name: "a revision activated several times records its latest predecessor",
			// r1 -> r2 -> r1 -> r2 -> r1.
			revs:        []Activation{act(1, 5, 2), act(2, 4, 1)},
			highest:     2,
			wantCurrent: 1, wantPrev: 2,
		},
		{
			name: "gaps in the numbers do not matter",
			// r1..r6 with r3 and r4 deleted; r6 replaced r5.
			revs:        []Activation{act(1, 1, 0), act(2, 2, 1), act(5, 5, 4), act(6, 6, 5)},
			highest:     6,
			wantCurrent: 6, wantPrev: 5,
		},
		{
			name: "the previously active revision was deleted: refused, never replaced by another",
			// r1 -> r2 -> r3; r2 deleted. r1 is NOT the answer.
			revs:        []Activation{act(1, 1, 0), act(3, 3, 2)},
			highest:     3,
			wantCurrent: 3, wantErr: ErrPreviousRevisionDeleted,
		},
		{
			name:        "the first activation has no previous revision",
			revs:        []Activation{act(1, 1, 0)},
			highest:     1,
			wantCurrent: 1, wantErr: ErrNoPreviousRevision,
		},
		{
			name:    "no revision at all",
			revs:    nil,
			wantErr: ErrNoActiveRevision,
		},
		{
			name:        "a corrupt annotation is refused",
			revs:        []Activation{act(1, 1, 0), withPrevious(act(2, 2, 0), "two")},
			highest:     2,
			wantCurrent: 2, wantErr: ErrPreviousRevisionUnknown,
		},
		{
			name:        "legacy history without gaps falls back to activation order",
			revs:        []Activation{act(1, 4, -1), act(2, 2, -1), act(3, 3, -1)},
			highest:     3,
			wantCurrent: 1, wantPrev: 3,
		},
		{
			name: "legacy history with a gap is inferred from activation order, and says so",
			// r2 may have been active between r3 and r1's reactivation.
			revs:        []Activation{act(1, 5, -1), act(3, 3, -1)},
			highest:     3,
			wantCurrent: 1, wantPrev: 3, wantInferred: true,
		},
		{
			name:        "legacy history with highestRevision unknown is inferred",
			revs:        []Activation{act(1, 1, -1), act(2, 2, -1)},
			highest:     0,
			wantCurrent: 2, wantPrev: 1, wantInferred: true,
		},
		{
			name:        "a recorded previous revision is never inferred, even with gaps",
			revs:        []Activation{act(1, 1, 0), act(4, 4, 1)},
			highest:     4,
			wantCurrent: 4, wantPrev: 1,
		},
		{
			name:        "legacy history whose activations tie is refused",
			revs:        []Activation{act(1, 2, -1), act(2, 2, -1), act(3, 3, -1)},
			highest:     3,
			wantCurrent: 3, wantErr: ErrPreviousRevisionUnknown,
		},
		{
			name:        "a legacy binding with its only revision has no previous revision",
			revs:        []Activation{act(1, 1, -1)},
			highest:     1,
			wantCurrent: 1, wantErr: ErrNoPreviousRevision,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			current, previous, inferred, err := PreviousActive(tt.revs, tt.highest)
			assert.Equal(t, tt.wantCurrent, current.Number, "current")
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, previous.Object)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantPrev, previous.Number)
			assert.Equal(t, tt.wantInferred, inferred != "", "inferred: %q", inferred)
		})
	}
}

func withPrevious(a Activation, raw string) Activation {
	a.Object.GetAnnotations()[AnnotationKeyPreviousRevision] = raw
	return a
}
