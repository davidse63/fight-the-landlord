package webgame

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/palemoky/fight-the-landlord/internal/bot"
	"github.com/palemoky/fight-the-landlord/internal/game/rule"
)

func TestNeuralFailureRetainsComplexMoves(t *testing.T) {
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer service.Close()
	r := testRoom()
	r.Phase, r.Landlord, r.Turn = "playing", 0, 0
	r.Players[0].Hand = ids("45678")
	r.Target, _ = parse(ids("34567"))
	engine := bot.NewDouZeroEngineWithFallback(service.URL, NewFallback())
	move := cardIDs(engine.DecidePlay(context.Background(), "test", r.botContext()))
	h, err := parse(move)
	if err != nil || h.Type != rule.Straight || !rule.CanBeat(h, r.Target) {
		t.Fatalf("neural outage lost legal straight: %v", move)
	}
}

// Optional real-model smoke test. Default test runs never download models.
func TestLiveDouZeroGames(t *testing.T) {
	url := os.Getenv("TEST_DOUZERO_URL")
	if url == "" {
		t.Skip("set TEST_DOUZERO_URL to test a running model service")
	}
	engine := bot.NewDouZeroEngineWithFallback(url, NewFallback())
	for trial := 0; trial < 10; trial++ {
		r := testRoom()
		for steps := 0; r.Phase != "ended" && steps < 350; steps++ {
			if r.Phase == "bidding" {
				r.bid(engine.DecideBid(context.Background(), "test", cards(r.Players[r.Turn].Hand), nil))
				continue
			}
			move := cardIDs(engine.DecidePlay(context.Background(), "test", r.botContext()))
			if err := r.play(move); err != nil {
				t.Fatalf("game %d illegal model move: %v", trial, err)
			}
		}
		if r.Phase != "ended" {
			t.Fatalf("game %d stalled", trial)
		}
	}
}
