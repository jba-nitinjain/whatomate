package handlers

import "strings"

const chatRepairDuplicateMoveConflictReason = "Multiple chats resolve to the same target phone number in the destination organization; review and merge them manually before applying repair"

type chatRepairMoveTargetKey struct {
	TargetOrgID string
	PhoneNumber string
}

func reconcileChatRepairCandidates(scannedContacts int64, candidates []ChatRepairCandidate) ([]ChatRepairCandidate, ChatRepairSummary) {
	moveTargets := make(map[chatRepairMoveTargetKey][]int)

	for i := range candidates {
		if candidates[i].Action != chatRepairActionMove {
			continue
		}

		key := chatRepairMoveTargetKey{
			TargetOrgID: strings.TrimSpace(candidates[i].TargetOrgID),
			PhoneNumber: strings.TrimSpace(candidates[i].PhoneNumber),
		}
		if key.TargetOrgID == "" || key.PhoneNumber == "" {
			continue
		}

		moveTargets[key] = append(moveTargets[key], i)
	}

	for _, indexes := range moveTargets {
		if len(indexes) < 2 {
			continue
		}
		for _, index := range indexes {
			candidates[index].Action = chatRepairActionConflict
			candidates[index].Reason = chatRepairDuplicateMoveConflictReason
			candidates[index].TargetContactID = ""
		}
	}

	summary := ChatRepairSummary{ScannedContacts: scannedContacts}
	for _, candidate := range candidates {
		summary.AffectedExternalMessages += candidate.AffectedMessageCount
		switch candidate.Action {
		case chatRepairActionMove:
			summary.MoveCandidates++
			summary.AutoFixableCandidates++
		case chatRepairActionMergeRequired:
			summary.MergeRequiredCandidates++
		case chatRepairActionConflict:
			summary.ConflictCandidates++
		}
	}

	return candidates, summary
}
