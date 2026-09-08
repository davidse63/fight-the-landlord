package webgame

import (
	"context"
	"encoding/json"
	"github.com/palemoky/fight-the-landlord/internal/game/card"
	"github.com/palemoky/fight-the-landlord/internal/game/rule"
	"reflect"
	"slices"
	"testing"
	"time"
)

func ids(text string) []int {
	h, e := card.FindCardsInHand(deck, text)
	if e != nil {
		panic(e)
	}
	return sorted(cardIDs(h))
}
func testRoom() *Room {
	r := newRoom("ABC234", true)
	for _, name := range []string{"A", "B", "C"} {
		r.Players = append(r.Players, &Player{ID: name, Name: name})
	}
	r.deal()
	return r
}
func TestEveryShapeCanBeSuggestedAndBeaten(t *testing.T) {
	for _, pair := range [][2]string{{"3", "4"}, {"33", "44"}, {"333", "444"}, {"3334", "4445"}, {"33344", "44455"}, {"34567", "45678"}, {"334455", "445566"}, {"333444", "444555"}, {"33344456", "44455567"}, {"33344455", "44455566"}, {"3334445566", "4445556677"}, {"333345", "444456"}, {"33334455", "44445566"}, {"3333", "4444"}, {"2222", "BR"}} {
		t.Run(pair[0], func(t *testing.T) {
			target, e := parse(ids(pair[0]))
			if e != nil {
				t.Fatal(e)
			}
			move := ids(pair[1])
			choices := legalMoves(move, target)
			found := false
			for _, c := range choices {
				if len(c) == len(move) {
					found = true
				}
			}
			if !found {
				t.Fatalf("no complete reply %s over %s: %v", pair[1], pair[0], choices)
			}
		})
	}
}
func TestRejectInvalidAndForgedMoves(t *testing.T) {
	r := testRoom()
	r.Phase = "playing"
	r.Landlord = 0
	r.Turn = 0
	for _, move := range [][]int{{54}, {-1}, {r.Players[0].Hand[0], r.Players[0].Hand[0]}, {r.Players[1].Hand[0]}, {}} {
		before := r.view("A")
		if r.play(move) == nil {
			t.Fatalf("accepted %v", move)
		}
		if !reflect.DeepEqual(before.Hand, r.view("A").Hand) {
			t.Fatal("invalid move changed hand")
		}
	}
	for _, bad := range []string{"3456", "TJQKA2", "333222", "33334444", "33334567"} {
		if _, e := parse(ids(bad)); e == nil {
			t.Fatalf("accepted invalid %s", bad)
		}
	}
}
func TestSnapshotDoesNotLeakCards(t *testing.T) {
	r := testRoom()
	v := r.view("A")
	if len(v.Bottom) != 0 || len(v.Hand) != 17 {
		t.Fatal("bidding snapshot exposes bottom or misses own hand")
	}
	for _, p := range v.Players {
		if len(p.Hand) != 0 {
			t.Fatal("snapshot exposed a hand")
		}
	}
	b, _ := json.Marshal(v)
	var wire map[string]any
	_ = json.Unmarshal(b, &wire)
	if _, ok := wire["token"]; ok {
		t.Fatal("token exposed")
	}
}
func TestBidPassesResetAndLeadReturns(t *testing.T) {
	r := testRoom()
	r.Turn = 0
	r.bid(true)
	r.bid(false)
	r.bid(false)
	if r.Landlord != 0 || r.Passes != 0 || len(r.Players[0].Hand) != 20 {
		t.Fatal("bad landlord transition")
	}
	move := []int{r.Players[0].Hand[0]}
	if e := r.play(move); e != nil {
		t.Fatal(e)
	}
	_ = r.play(nil)
	_ = r.play(nil)
	if r.Turn != 0 || !r.Target.IsEmpty() {
		t.Fatal("lead failed to return")
	}
}
func TestGamesFinishWithConservationAndZeroSum(t *testing.T) {
	engine := NewFallback()
	for trial := 0; trial < 40; trial++ {
		r := testRoom()
		for steps := 0; r.Phase != "ended" && steps < 350; steps++ {
			if r.Phase == "bidding" {
				r.bid(engine.DecideBid(context.Background(), "", cards(r.Players[r.Turn].Hand), nil))
				continue
			}
			move := cardIDs(engine.DecidePlay(context.Background(), "", r.botContext()))
			if e := r.play(move); e != nil {
				t.Fatalf("trial %d: %v", trial, e)
			}
			seen := map[int]bool{}
			for _, p := range r.Players {
				for _, id := range append(slices.Clone(p.Hand), p.Played...) {
					if seen[id] {
						t.Fatal("card duplicated")
					}
					seen[id] = true
				}
			}
			if len(seen) != 54 {
				t.Fatalf("lost cards: %d", len(seen))
			}
		}
		if r.Phase != "ended" {
			t.Fatal("game stalled")
		}
		score := 0
		for _, p := range r.Players {
			score += p.Score
		}
		if score != 0 {
			t.Fatal("score not zero sum")
		}
	}
}
func TestSpringAndCounterSpring(t *testing.T) {
	for _, landlordWins := range []bool{true, false} {
		r := testRoom()
		r.Phase = "playing"
		r.Landlord = 0
		r.Multiplier = 2
		r.Turn = 0
		r.Players[0].Hand = ids("3")
		if !landlordWins {
			r.Turn = 1
			r.PlayCounts[0] = 1
			r.Players[1].Hand = ids("3")
		}
		if e := r.play(ids("3")); e != nil {
			t.Fatal(e)
		}
		if !r.Spring || r.Multiplier != 4 {
			t.Fatal("spring not applied")
		}
	}
}
func TestReconnectMetadataPreservesDeadline(t *testing.T) {
	s := New(nil, NewFallback(), false)
	r := testRoom()
	r.Practice = false
	r.Phase = "playing"
	r.Landlord = 0
	s.rooms[r.Code] = r
	s.changed(r)
	deadline := r.Deadline
	s.changed(r, true)
	defer r.Timer.Stop()
	if d := r.Deadline.Sub(deadline); d > time.Millisecond || d < -time.Millisecond {
		t.Fatalf("metadata reset turn clock by %v", d)
	}
}
func TestSuggestionSetMatchesBruteForce(t *testing.T) {
	hand := ids("33344455667789")
	target := rule.ParsedHand{}
	moves := legalMoves(hand, target)
	keys := map[string]bool{}
	for _, m := range moves {
		keys[ranksText(m)] = true
	}
	for mask := 1; mask < (1 << len(hand)); mask++ {
		move := []int{}
		for i, id := range hand {
			if mask&(1<<i) != 0 {
				move = append(move, id)
			}
		}
		if _, e := parse(move); e == nil && !keys[ranksText(move)] {
			t.Fatalf("missing legal combination: %s", ranksText(move))
		}
	}
}
