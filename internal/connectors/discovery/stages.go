package discovery

import "encoding/json"

// Tier maps (confidence, source, userCustom) to a review tier. userCustom
// covers Flexi-style customer-defined fields (userField*) which always land
// in the review queue regardless of confidence.
func Tier(confidence float64, source Source, userCustom bool) Review {
	if userCustom {
		return ReviewNeedsReview
	}
	switch {
	case confidence >= 0.95:
		return ReviewAuto
	case confidence >= 0.7:
		return ReviewAutoLowConfidence
	default:
		return ReviewNeedsReview
	}
}

func ProposeEntities(art *Artifact) {
	for _, ev := range art.Evidences {
		if !PromotableRoles[ev.Role] {
			continue
		}
		if ev.OpenRowEntity != nil {
			continue
		}
		ev.OpenRowEntity = &EntityProposal{
			Name:        string(ev.Role),
			DisplayName: defaultDisplayName(ev.Role),
		}
	}
}

func defaultDisplayName(r Role) string {
	switch r {
	case RoleCustomer:
		return "Zákazník"
	case RoleSupplier:
		return "Dodavatel"
	case RoleProduct:
		return "Produkt"
	case RoleProject:
		return "Zakázka"
	case RoleCostCenter:
		return "Středisko"
	case RoleBankAccount:
		return "Bankovní účet"
	}
	return string(r)
}

func BuildReviewItems(art *Artifact) []ReviewItem {
	out := []ReviewItem{}
	for evName, ev := range art.Evidences {
		if ev.Review == ReviewNeedsReview {
			out = append(out, ReviewItem{
				Evidence: evName,
				Proposed: mustJSON(ev),
				Status:   "pending",
			})
		}
		for fName, f := range ev.Fields {
			if f.Review == ReviewNeedsReview {
				out = append(out, ReviewItem{
					Evidence: evName,
					Field:    fName,
					Proposed: mustJSON(f),
					Status:   "pending",
				})
			}
		}
	}
	return out
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
