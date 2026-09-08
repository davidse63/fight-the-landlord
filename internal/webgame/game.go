// SPDX-License-Identifier: GPL-3.0-or-later
package webgame

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"github.com/palemoky/fight-the-landlord/internal/bot"
	"github.com/palemoky/fight-the-landlord/internal/game/card"
	"github.com/palemoky/fight-the-landlord/internal/game/rule"
	"math/big"
	"slices"
	"time"
)

type Player struct {
	ID     string
	Name   string
	Bot    bool
	Ready  bool
	Auto   bool
	Hand   []int
	Played []int
	Last   []int
	Say    string
	Wins   int
	Score  int
	Conn   *peer
}
type Event struct {
	Seat  int    `json:"seat"`
	Text  string `json:"text"`
	Cards []int  `json:"cards,omitempty"`
}
type Room struct {
	Code       string
	Practice   bool
	Players    []*Player
	Phase      string
	Turn       int
	Landlord   int
	Candidate  int
	Caller     int
	Passes     int
	Grabs      int
	Multiplier int
	Bottom     []int
	Target     rule.ParsedHand
	LastSeat   int
	Winner     int
	Version    int
	Round      int
	Deadline   time.Time
	Touched    time.Time
	Timer      *time.Timer
	History    []Event
	Actions    [][]card.Rank
	PlayCounts [3]int
	Spring     bool
}
type PlayerView struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Bot    bool   `json:"bot"`
	Ready  bool   `json:"ready"`
	Auto   bool   `json:"auto"`
	Online bool   `json:"online"`
	Count  int    `json:"count"`
	Last   []int  `json:"last"`
	Say    string `json:"say"`
	Hand   []int  `json:"hand,omitempty"`
	Score  int    `json:"score"`
}
type View struct {
	Code       string       `json:"code"`
	Practice   bool         `json:"practice"`
	Phase      string       `json:"phase"`
	You        int          `json:"you"`
	Players    []PlayerView `json:"players"`
	Hand       []int        `json:"hand"`
	Bottom     []int        `json:"bottom"`
	Turn       int          `json:"turn"`
	Landlord   int          `json:"landlord"`
	Grab       bool         `json:"grab"`
	Multiplier int          `json:"multiplier"`
	Target     []int        `json:"target"`
	LastSeat   int          `json:"lastSeat"`
	MustPlay   bool         `json:"mustPlay"`
	Version    int          `json:"version"`
	Round      int          `json:"round"`
	Deadline   int64        `json:"deadline"`
	ServerTime int64        `json:"serverTime"`
	Winner     int          `json:"winner"`
	Spring     bool         `json:"spring"`
	History    []Event      `json:"history"`
}

func randomID() string {
	var b [24]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b[:])
}
func randomN(n int) int {
	v, e := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if e != nil {
		panic(e)
	}
	return int(v.Int64())
}
func newRoom(code string, practice bool) *Room {
	return &Room{Code: code, Practice: practice, Phase: "waiting", Landlord: -1, Winner: -1, LastSeat: -1, Touched: time.Now()}
}
func (r *Room) seat(id string) int {
	for i, p := range r.Players {
		if p.ID == id {
			return i
		}
	}
	return -1
}
func (r *Room) event(seat int, text string, ids []int) {
	r.History = append(r.History, Event{seat, text, slices.Clone(ids)})
}
func (r *Room) deal() {
	r.Round++
	r.Phase = "bidding"
	r.Bottom = nil
	r.Landlord = -1
	r.Candidate = -1
	r.Caller = -1
	r.Passes = 0
	r.Grabs = 0
	r.Multiplier = 1
	r.Target = rule.ParsedHand{}
	r.LastSeat = -1
	r.Winner = -1
	r.Spring = false
	r.History = nil
	r.Actions = nil
	r.PlayCounts = [3]int{}
	r.Turn = randomN(3)
	ids := make([]int, 54)
	for i := range ids {
		ids[i] = i
	}
	for i := 53; i > 0; i-- {
		j := randomN(i + 1)
		ids[i], ids[j] = ids[j], ids[i]
	}
	for i, p := range r.Players {
		p.Hand = sorted(ids[i*17 : (i+1)*17])
		p.Last = nil
		p.Played = nil
		p.Say = ""
		p.Score = 0
	}
	r.Bottom = slices.Clone(ids[51:])
	r.event(-1, "新一局开始，请依次叫地主", nil)
}
func (r *Room) bid(yes bool) {
	p := r.Players[r.Turn]
	grab := r.Candidate >= 0
	word := "不叫"
	if grab {
		word = "不抢"
	}
	if yes {
		word = "叫地主"
		if grab {
			word = "抢地主"
			r.Multiplier *= 2
		} else {
			r.Caller = r.Turn
		}
		r.Candidate = r.Turn
		r.Passes = 0
	} else {
		r.Passes++
	}
	p.Say = word
	r.event(r.Turn, word, nil)
	if !grab && !yes && r.Passes == 3 {
		r.deal()
		r.event(-1, "本轮无人叫地主，已重新洗牌", nil)
		return
	}
	if grab {
		r.Grabs++
		if r.Passes >= 2 || r.Grabs >= 3 {
			r.Landlord = r.Candidate
			r.Turn = r.Landlord
			r.Phase = "playing"
			r.Passes = 0
			r.Players[r.Landlord].Hand = sorted(append(r.Players[r.Landlord].Hand, r.Bottom...))
			for _, q := range r.Players {
				q.Say = ""
			}
			r.event(r.Landlord, "成为地主", nil)
			return
		}
	}
	r.Turn = (r.Turn + 1) % 3
	if r.Candidate == r.Turn {
		r.Turn = (r.Turn + 1) % 3
	}
}
func (r *Room) play(move []int) error {
	p := r.Players[r.Turn]
	if len(move) == 0 {
		if r.Target.IsEmpty() {
			return errors.New("新一轮由你领出，不能不出")
		}
		p.Last = nil
		p.Say = "不出"
		r.event(r.Turn, "不出", nil)
		r.Actions = append(r.Actions, nil)
		r.Passes++
		r.Turn = (r.Turn + 1) % 3
		if r.Passes == 2 {
			r.Turn = r.LastSeat
			r.Target = rule.ParsedHand{}
			r.Passes = 0
			for _, q := range r.Players {
				q.Last = nil
				q.Say = ""
			}
			r.event(-1, "两家不出，重新领出", nil)
		}
		return nil
	}
	if !owns(p.Hand, move) {
		return errors.New("所选牌不在你的手中，或有重复牌")
	}
	h, err := parse(move)
	if err != nil {
		return errors.New("这些牌不能组成有效牌型，请调整选择")
	}
	if !r.Target.IsEmpty() && !rule.CanBeat(h, r.Target) {
		return errors.New("需用相同牌型和长度压过上家，或使用炸弹、王炸")
	}
	p.Hand = cardIDs(card.RemoveCards(cards(p.Hand), cards(move)))
	p.Last = sorted(move)
	p.Say = h.Type.String()
	p.Played = append(p.Played, move...)
	r.Target = h
	r.LastSeat = r.Turn
	r.Passes = 0
	r.PlayCounts[r.Turn]++
	r.event(r.Turn, h.Type.String(), sorted(move))
	rr := []card.Rank{}
	for _, c := range cards(move) {
		rr = append(rr, c.Rank)
	}
	r.Actions = append(r.Actions, rr)
	if h.Type == rule.Bomb || h.Type == rule.Rocket {
		r.Multiplier *= 2
	}
	if len(p.Hand) == 0 {
		r.Winner = r.Turn
		r.Phase = "ended"
		farmerPlays := 0
		for i, n := range r.PlayCounts {
			if i != r.Landlord {
				farmerPlays += n
			}
		}
		r.Spring = (r.Winner == r.Landlord && farmerPlays == 0) || (r.Winner != r.Landlord && r.PlayCounts[r.Landlord] == 1)
		if r.Spring {
			r.Multiplier *= 2
		}
		for i, q := range r.Players {
			win := (i == r.Landlord) == (r.Winner == r.Landlord)
			q.Score = r.Multiplier
			if i == r.Landlord {
				q.Score *= 2
			}
			if !win {
				q.Score = -q.Score
			} else {
				q.Wins++
			}
			q.Ready = false
			q.Auto = false
		}
		return nil
	}
	r.Turn = (r.Turn + 1) % 3
	return nil
}
func (r *Room) view(id string) View {
	you := r.seat(id)
	v := View{Code: r.Code, Practice: r.Practice, Phase: r.Phase, You: you, Turn: r.Turn, Landlord: r.Landlord, Grab: r.Candidate >= 0, Multiplier: r.Multiplier, Target: cardIDs(r.Target.Cards), LastSeat: r.LastSeat, MustPlay: r.Target.IsEmpty(), Version: r.Version, Round: r.Round, ServerTime: time.Now().UnixMilli(), Winner: r.Winner, Spring: r.Spring, History: r.History}
	if !r.Deadline.IsZero() {
		v.Deadline = r.Deadline.UnixMilli()
	}
	if you >= 0 {
		v.Hand = slices.Clone(r.Players[you].Hand)
	}
	if r.Landlord >= 0 {
		v.Bottom = slices.Clone(r.Bottom)
	}
	for _, p := range r.Players {
		pv := PlayerView{ID: p.ID, Name: p.Name, Bot: p.Bot, Ready: p.Ready, Auto: p.Auto, Online: p.Bot || p.Conn != nil, Count: len(p.Hand), Last: p.Last, Say: p.Say, Score: p.Score}
		if r.Phase == "ended" {
			pv.Hand = p.Hand
		}
		v.Players = append(v.Players, pv)
	}
	return v
}
func (r *Room) botContext() bot.GameContext {
	seat := r.Turn
	p := r.Players[seat]
	positions := []string{bot.DouZeroPosLandlord, bot.DouZeroPosLandlordDn, bot.DouZeroPosLandlordUp}
	pos := func(s int) int { return (s - r.Landlord + 3) % 3 }
	g := bot.GameContext{Hand: cards(p.Hand), BottomCards: cards(r.Bottom), IsLandlord: seat == r.Landlord, MustPlay: r.Target.IsEmpty(), CanBeat: len(legalMoves(p.Hand, r.Target)) > 0, DouZeroPos: positions[pos(seat)], ActionSeq: r.Actions, NumCardsLeft: map[string]int{}}
	g.RecentPlays[0] = bot.PlayRecord{Played: r.Target}
	if r.LastSeat >= 0 {
		g.LastMovePos = positions[pos(r.LastSeat)]
		g.RecentPlays[0].IsLandlord = r.LastSeat == r.Landlord
	}
	for i, q := range r.Players {
		g.NumCardsLeft[positions[pos(i)]] = len(q.Hand)
		for _, id := range q.Played {
			g.PlayedByPos[pos(i)] = append(g.PlayedByPos[pos(i)], deck[id].Rank)
		}
	}
	return g
}
