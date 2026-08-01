package services

import (
	"testing"
	"time"
)

func TestBackoffRespectsMaxDelay(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:  5,
		BaseDelay:   time.Second,
		MaxDelay:    4 * time.Second,
		ServiceName: "test",
	}
	delay := backoffDelay(&cfg, 3)
	if delay > 5*time.Second {
		t.Errorf("delay %v exceeds expected max", delay)
	}
	if delay < 4*time.Second {
		t.Errorf("delay %v too small for attempt 3", delay)
	}
}

func TestBackoffFirstAttempt(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:  2,
		BaseDelay:   500 * time.Millisecond,
		MaxDelay:    4 * time.Second,
		ServiceName: "test",
	}
	delay := backoffDelay(&cfg, 0)
	// Base delay + up to 25% jitter
	if delay < 500*time.Millisecond {
		t.Errorf("delay %v less than base", delay)
	}
	if delay > 625*time.Millisecond {
		t.Errorf("delay %v exceeds base + 25%% jitter", delay)
	}
}

func TestBackoffExponentialGrowth(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:  5,
		BaseDelay:   time.Second,
		MaxDelay:    time.Minute,
		ServiceName: "test",
	}
	var results []time.Duration
	for i := uint32(0); i <= 3; i++ {
		results = append(results, backoffDelay(&cfg, i).Round(time.Second))
	}
	// Each subsequent should be roughly double
	if results[1] < results[0] {
		t.Error("delay should increase with each attempt")
	}
}

func TestLangBase(t *testing.T) {
	if LangBase("pt-BR") != "pt" {
		t.Errorf("expected pt, got %s", LangBase("pt-BR"))
	}
	if LangBase("en") != "en" {
		t.Errorf("expected en, got %s", LangBase("en"))
	}
}

func TestLangRegion(t *testing.T) {
	if LangRegion("pt-BR") != "BR" {
		t.Errorf("expected BR, got %s", LangRegion("pt-BR"))
	}
	if LangRegion("en") != "" {
		t.Errorf("expected empty, got %s", LangRegion("en"))
	}
}
