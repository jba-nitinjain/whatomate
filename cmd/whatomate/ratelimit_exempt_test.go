package main

import "testing"

func TestIsRateLimitExemptProviderCallback(t *testing.T) {
	exempt := []string{"/api/webhook", "/api/webhook/", "/api/webhook/leadgen", "/api/webhook/leadgen/"}
	for _, p := range exempt {
		if !isRateLimitExemptProviderCallback(p) {
			t.Errorf("%s should bypass the API rate limiter", p)
		}
	}
	limited := []string{"/api/webhooks", "/api/webhooks/123/test", "/api/contacts", "/api/webhook-other", "/api/messages/template"}
	for _, p := range limited {
		if isRateLimitExemptProviderCallback(p) {
			t.Errorf("%s must stay rate limited", p)
		}
	}
}
