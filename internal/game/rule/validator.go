package rule

import (
	"github.com/palemoky/fight-the-landlord/internal/game/card"
)

// isRocket 王炸
func isRocket(analysis HandAnalysis, cards []card.Card) (ParsedHand, bool) {
	if len(cards) == 2 && analysis.counts[card.RankBlackJoker] == 1 && analysis.counts[card.RankRedJoker] == 1 {
		return ParsedHand{Type: Rocket, KeyRank: card.RankRedJoker, Cards: cards}, true
	}
	return ParsedHand{}, false
}

// isBomb 炸弹
func isBomb(analysis HandAnalysis, cards []card.Card) (ParsedHand, bool) {
	if len(analysis.fours) == 1 && len(cards) == 4 {
		return ParsedHand{Type: Bomb, KeyRank: analysis.fours[0], Cards: cards}, true
	}
	return ParsedHand{}, false
}

// isFourWithKickers 四带二
func isFourWithKickers(analysis HandAnalysis, cards []card.Card) (ParsedHand, bool) {
	cardLen := len(cards)
	if len(analysis.fours) == 1 {
		hand := ParsedHand{KeyRank: analysis.fours[0], Cards: cards}
		if cardLen == 6 && (len(analysis.ones) == 2 || len(analysis.pairs) == 1) { // AAAABB、AAAABC
			// 四带二，可以带两张单牌，也可以带一个对子(不算四带两对)
			hand.Type = FourWithTwo
			return hand, true
		}
		if cardLen == 8 && len(analysis.pairs) == 2 { // AAAABBCC、AAAABBBB
			hand.Type = FourWithTwoPairs
			return hand, true
		}
	}
	return ParsedHand{}, false
}

// isTrioWithKickers 三带X
func isTrioWithKickers(analysis HandAnalysis, cards []card.Card) (ParsedHand, bool) {
	cardLen := len(cards)
	if len(analysis.trios) == 1 {
		hand := ParsedHand{KeyRank: analysis.trios[0], Cards: cards}
		if cardLen == 4 && len(analysis.ones) == 1 { // AAAB
			hand.Type = TrioWithSingle
			return hand, true
		}
		if cardLen == 5 && len(analysis.pairs) == 1 { // AAABB
			hand.Type = TrioWithPair
			return hand, true
		}
	}
	return ParsedHand{}, false
}

// isPlane 飞机
func isPlane(analysis HandAnalysis, cards []card.Card) (ParsedHand, bool) {
	// The body cannot contain 2/jokers or reuse its own rank as wings.
	// Single wings may themselves form pairs, as in 33344455.
	for _, width := range []int{3, 4, 5} {
		if len(cards)%width != 0 || len(cards)/width < 2 {
			continue
		}
		n := len(cards) / width
		for start := card.Rank3; start+card.Rank(n)-1 <= card.RankA; start++ {
			valid := true
			for r := start; r < start+card.Rank(n); r++ {
				if analysis.counts[r] != 3 {
					valid = false
					break
				}
			}
			if !valid {
				continue
			}
			if width == 5 {
				pairs := 0
				for r, c := range analysis.counts {
					if r >= start && r < start+card.Rank(n) {
						continue
					}
					if c != 2 {
						valid = false
						break
					}
					pairs++
				}
				valid = valid && pairs == n
			}
			if valid {
				kind := Plane
				if width == 4 {
					kind = PlaneWithSingles
				}
				if width == 5 {
					kind = PlaneWithPairs
				}
				return ParsedHand{Type: kind, KeyRank: start, Length: n, Cards: cards}, true
			}
		}
	}
	return ParsedHand{}, false
}

// isStraight 顺子
func isStraight(analysis HandAnalysis, cards []card.Card) (ParsedHand, bool) {
	cardLen := len(cards)
	if isContinuous(analysis.ones) && len(analysis.ones) == cardLen && cardLen >= 5 { // ABCDE+
		return ParsedHand{Type: Straight, KeyRank: analysis.ones[0], Length: cardLen, Cards: cards}, true
	}
	return ParsedHand{}, false
}

// isPairStraight 连对
func isPairStraight(analysis HandAnalysis, cards []card.Card) (ParsedHand, bool) {
	if isContinuous(analysis.pairs) && len(analysis.pairs)*2 == len(cards) && len(analysis.pairs) >= 3 { // AABBCC+
		return ParsedHand{Type: PairStraight, KeyRank: analysis.pairs[0], Length: len(analysis.pairs), Cards: cards}, true
	}
	return ParsedHand{}, false
}

// isSimpleType 简单牌型：单、对、三
func isSimpleType(analysis HandAnalysis, cards []card.Card) (ParsedHand, bool) {
	if len(analysis.counts) == 1 {
		switch len(cards) {
		case 1:
			return ParsedHand{Type: Single, KeyRank: analysis.ones[0], Cards: cards}, true
		case 2:
			return ParsedHand{Type: Pair, KeyRank: analysis.pairs[0], Cards: cards}, true
		case 3:
			return ParsedHand{Type: Trio, KeyRank: analysis.trios[0], Cards: cards}, true
		}
	}
	return ParsedHand{}, false
}
