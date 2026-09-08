// SPDX-License-Identifier: GPL-3.0-or-later
package webgame

import (
	"context"
	"github.com/palemoky/fight-the-landlord/internal/bot"
	"github.com/palemoky/fight-the-landlord/internal/game/card"
)

type fallback struct{ bid *bot.HeuristicEngine }

func NewFallback() bot.DecisionEngine { return &fallback{bot.NewHeuristicEngine()} }
func (f *fallback) DecideBid(ctx context.Context, name string, hand []card.Card, prev *bool) bool {
	return f.bid.DecideBid(ctx, name, hand, prev)
}
func (f *fallback) DecidePlay(_ context.Context, _ string, g bot.GameContext) []card.Card {
	moves := legalMoves(cardIDs(g.Hand), g.RecentPlays[0].Played)
	if len(moves) == 0 {
		return nil
	}
	best := moves[0]
	if !g.MustPlay && !g.IsLandlord && !g.RecentPlays[0].IsLandlord && len(best) != len(g.Hand) {
		return nil
	}
	return cards(best)
}
