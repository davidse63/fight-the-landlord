// SPDX-License-Identifier: GPL-3.0-or-later
package webgame

import (
	"cmp"
	"github.com/palemoky/fight-the-landlord/internal/game/card"
	"github.com/palemoky/fight-the-landlord/internal/game/rule"
	"slices"
	"strconv"
	"strings"
)

var deck = card.NewDeck()

func cards(ids []int) []card.Card {
	out := make([]card.Card, len(ids))
	for i, id := range ids {
		out[i] = deck[id]
	}
	return out
}
func cardIDs(cs []card.Card) []int {
	ids := make([]int, 0, len(cs))
	for _, c := range cs {
		for id, d := range deck {
			if c == d {
				ids = append(ids, id)
				break
			}
		}
	}
	return ids
}
func sorted(ids []int) []int {
	ids = slices.Clone(ids)
	slices.SortFunc(ids, func(a, b int) int {
		if deck[a].Rank != deck[b].Rank {
			return cmp.Compare(deck[b].Rank, deck[a].Rank)
		}
		return cmp.Compare(a, b)
	})
	return ids
}
func owns(hand, move []int) bool {
	seen := map[int]bool{}
	for _, id := range move {
		if id < 0 || id >= 54 || seen[id] || !slices.Contains(hand, id) {
			return false
		}
		seen[id] = true
	}
	return true
}
func parse(ids []int) (rule.ParsedHand, error) { return rule.ParseHand(cards(ids)) }

// Generate rank combinations, not suit permutations. Every candidate is checked
// by the same parser used to accept human moves.
func legalMoves(hand []int, target rule.ParsedHand) [][]int {
	groups := map[int][]int{}
	for _, id := range sorted(hand) {
		r := int(deck[id].Rank)
		groups[r] = append(groups[r], id)
	}
	result := [][]int{}
	seen := map[string]bool{}
	add := func(ids []int) {
		h, err := parse(ids)
		if err != nil || (!target.IsEmpty() && !rule.CanBeat(h, target)) {
			return
		}
		key := ""
		for _, id := range sorted(ids) {
			key += strconv.Itoa(int(deck[id].Rank)) + ","
		}
		if !seen[key] {
			seen[key] = true
			result = append(result, slices.Clone(ids))
		}
	}
	// Attach n single cards (duplicates allowed) or n distinct pairs, excluding body ranks.
	attach := func(body []int, n, unit int) {
		excluded := map[int]bool{}
		for _, id := range body {
			excluded[int(deck[id].Rank)] = true
		}
		var walk func(int, int, []int)
		walk = func(rank, left int, acc []int) {
			if left == 0 {
				add(append(slices.Clone(body), acc...))
				return
			}
			if rank > 17 {
				return
			}
			walk(rank+1, left, acc)
			if excluded[rank] {
				return
			}
			limit := min(left, len(groups[rank])/unit)
			if unit == 2 {
				limit = min(1, limit)
			}
			for count := 1; count <= limit; count++ {
				walk(rank+1, left-count, append(slices.Clone(acc), groups[rank][:count*unit]...))
			}
		}
		walk(3, n, nil)
	}
	for rank := 3; rank <= 17; rank++ {
		g := groups[rank]
		for n := 1; n <= min(4, len(g)); n++ {
			base := g[:n]
			add(base)
			if n == 3 {
				attach(base, 1, 1)
				attach(base, 1, 2)
			}
			if n == 4 {
				attach(base, 2, 1)
				attach(base, 2, 2)
			}
		}
	}
	if len(groups[16]) > 0 && len(groups[17]) > 0 {
		add([]int{groups[16][0], groups[17][0]})
	}
	for unit := 1; unit <= 3; unit++ {
		minimum := map[int]int{1: 5, 2: 3, 3: 2}[unit]
		for start := 3; start <= 14; start++ {
			body := []int{}
			for end := start; end <= 14 && len(groups[end]) >= unit; end++ {
				body = append(body, groups[end][:unit]...)
				length := end - start + 1
				if length < minimum {
					continue
				}
				add(body)
				if unit == 3 {
					attach(body, length, 1)
					attach(body, length, 2)
				}
			}
		}
	}
	slices.SortFunc(result, func(a, b []int) int { return cmp.Compare(moveCost(a, hand, target), moveCost(b, hand, target)) })
	return result
}

func moveCost(move, hand []int, target rule.ParsedHand) int {
	h, _ := parse(move)
	cost := int(h.KeyRank)
	if target.IsEmpty() {
		cost -= len(move) * 25
	}
	if len(move) == len(hand) {
		return -10000
	}
	if h.Type == rule.Bomb || h.Type == rule.Rocket {
		cost += 160
	}
	counts := map[card.Rank]int{}
	used := map[card.Rank]int{}
	for _, id := range hand {
		counts[deck[id].Rank]++
	}
	for _, id := range move {
		used[deck[id].Rank]++
	}
	for rank, n := range used {
		if n < counts[rank] {
			cost += 10 * (counts[rank] - n)
			if counts[rank] == 4 {
				cost += 80
			}
		}
	}
	return cost
}
func typeName(ids []int) string {
	h, e := parse(ids)
	if e != nil {
		return ""
	}
	return h.Type.String()
}
func ranksText(ids []int) string {
	var ss []string
	for _, id := range sorted(ids) {
		r := deck[id].Rank.String()
		if r == "B" {
			r = "小王"
		}
		if r == "R" {
			r = "大王"
		}
		ss = append(ss, r)
	}
	return strings.Join(ss, " ")
}
