package discovery

import "testing"

func TestTier(t *testing.T) {
	t.Parallel()
	cases := []struct {
		conf       float64
		source     Source
		userCustom bool
		want       Review
	}{
		{1.0, SourceHeuristic, false, ReviewAuto},
		{0.95, SourceLLM, false, ReviewAuto},
		{0.85, SourceLLM, false, ReviewAutoLowConfidence},
		{0.6, SourceLLM, false, ReviewNeedsReview},
		{0.99, SourceLLM, true, ReviewNeedsReview},
	}
	for _, tc := range cases {
		got := Tier(tc.conf, tc.source, tc.userCustom)
		if got != tc.want {
			t.Errorf("Tier(%v,%v,%v) = %v, want %v", tc.conf, tc.source, tc.userCustom, got, tc.want)
		}
	}
}

func TestProposeEntities(t *testing.T) {
	t.Parallel()
	art := &Artifact{
		Evidences: map[string]*Evidence{
			"adresar":        {Role: RoleCustomer, Confidence: 1.0, Review: ReviewAuto},
			"faktura-vydana": {Role: RoleInvoiceOutgoing, Confidence: 1.0, Review: ReviewAuto},
		},
	}
	ProposeEntities(art)
	if art.Evidences["adresar"].OpenRowEntity == nil {
		t.Errorf("customer should get an entity proposal")
	}
	if art.Evidences["faktura-vydana"].OpenRowEntity != nil {
		t.Errorf("invoice (transactional) should not be promoted")
	}
}

func TestBuildReviewItems(t *testing.T) {
	t.Parallel()
	art := &Artifact{
		Evidences: map[string]*Evidence{
			"weird": {Role: RoleUnknown, Confidence: 0.4, Review: ReviewNeedsReview,
				Fields: map[string]*Field{
					"foo": {Role: RoleUnknown, Confidence: 0.3, Review: ReviewNeedsReview},
					"bar": {Role: RoleDisplayName, Confidence: 1.0, Review: ReviewAuto},
				}},
			"adresar": {Role: RoleCustomer, Confidence: 1.0, Review: ReviewAuto,
				Fields: map[string]*Field{
					"userField1@FX001": {Role: RoleUnknown, Confidence: 0.5, Review: ReviewNeedsReview},
				}},
		},
	}
	items := BuildReviewItems(art)
	if len(items) != 3 {
		t.Fatalf("got %d items, want 3 (%+v)", len(items), items)
	}
}
