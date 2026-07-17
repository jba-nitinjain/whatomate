package handlers

// normalizeRSVPReminderPhone reduces a phone to comparable digits using the same
// rule as RSVP capture (rsvp_capture.go): strip to digits, and prefix a bare
// 10-digit Indian mobile with 91 so it matches the format WhatsApp reports.
//
// campaigns.go's normalizeCampaignRecipientPhone is deliberately not reused: it
// only trims whitespace and a leading "+", so it cannot merge 9840445616 with
// 919840445616.
func normalizeRSVPReminderPhone(phone string) string {
	digits := normalizePhoneDigits(phone)
	if len(digits) == 10 {
		return "91" + digits
	}
	return digits
}

// dedupeRSVPReminderRows removes rows whose phone normalizes to one already seen,
// keeping the first occurrence. Rows that normalize to "" are kept, not dropped —
// the caller records them as skipped with a reason rather than losing them.
func dedupeRSVPReminderRows[T any](rows []T, phoneOf func(T) string) (kept []T, dropped []T) {
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		key := normalizeRSVPReminderPhone(phoneOf(row))
		if key == "" {
			kept = append(kept, row)
			continue
		}
		if _, dup := seen[key]; dup {
			dropped = append(dropped, row)
			continue
		}
		seen[key] = struct{}{}
		kept = append(kept, row)
	}
	return kept, dropped
}
