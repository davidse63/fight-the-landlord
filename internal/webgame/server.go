// SPDX-License-Identifier: GPL-3.0-or-later
package webgame

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/palemoky/fight-the-landlord/internal/bot"
	"github.com/palemoky/fight-the-landlord/internal/game/rule"
	"io/fs"
	"log"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

type guest struct {
	Player *Player
	Room   string
	Seen   time.Time
}
type peer struct {
	conn  *websocket.Conn
	send  chan []byte
	done  chan struct{}
	once  sync.Once
	token string
	ip    string
}

func (p *peer) close() { p.once.Do(func() { close(p.done); _ = p.conn.Close() }) }
func (p *peer) push(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	select {
	case p.send <- b:
	default:
		p.close()
	}
}
func (p *peer) writer() {
	defer p.close()
	tick := time.NewTicker(25 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-p.done:
			return
		case b := <-p.send:
			_ = p.conn.SetWriteDeadline(time.Now().Add(8 * time.Second))
			if p.conn.WriteMessage(websocket.TextMessage, b) != nil {
				return
			}
		case <-tick.C:
			if p.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(8*time.Second)) != nil {
				return
			}
		}
	}
}

type request struct {
	Type    string `json:"type"`
	Token   string `json:"token"`
	Code    string `json:"code"`
	Cards   []int  `json:"cards"`
	Bid     bool   `json:"bid"`
	Auto    bool   `json:"auto"`
	Version int    `json:"version"`
	Index   int    `json:"index"`
}
type Server struct {
	mu          sync.Mutex
	rooms       map[string]*Room
	guests      map[string]*guest
	connections map[string]int
	origins     map[string]bool
	engine      bot.DecisionEngine
	trustProxy  bool
	botDelay    time.Duration
}

func New(origins []string, engine bot.DecisionEngine, trustProxy bool) *Server {
	s := &Server{rooms: map[string]*Room{}, guests: map[string]*guest{}, connections: map[string]int{}, origins: map[string]bool{}, engine: engine, trustProxy: trustProxy, botDelay: 1200 * time.Millisecond}
	for _, o := range origins {
		s.origins[strings.TrimSpace(o)] = true
	}
	return s
}
func (s *Server) Handler(assets fs.FS) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.socket)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.Handle("/", http.FileServerFS(assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors https://fengxun.ca https://www.fengxun.ca http://localhost:4347 http://127.0.0.1:4347; base-uri 'none'; form-action 'self'")
		mux.ServeHTTP(w, r)
	})
}
func (s *Server) socket(w http.ResponseWriter, r *http.Request) {
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	if s.trustProxy && net.ParseIP(r.Header.Get("CF-Connecting-IP")) != nil {
		ip = r.Header.Get("CF-Connecting-IP")
	}
	s.mu.Lock()
	if s.connections[ip] >= 9 {
		s.mu.Unlock()
		http.Error(w, "Too many connections", 429)
		return
	}
	s.connections[ip]++
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.connections[ip]--
		if s.connections[ip] == 0 {
			delete(s.connections, ip)
		}
		s.mu.Unlock()
	}()
	up := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return s.origins[r.Header.Get("Origin")] }}
	conn, err := up.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	p := &peer{conn: conn, send: make(chan []byte, 24), done: make(chan struct{}), ip: ip}
	defer p.close()
	conn.SetReadLimit(4096)
	_ = conn.SetReadDeadline(time.Now().Add(8 * time.Second))
	var hello request
	if conn.ReadJSON(&hello) != nil || hello.Type != "hello" {
		return
	}
	go p.writer()
	s.mu.Lock()
	s.cleanupLocked()
	g := s.guests[hello.Token]
	resumed := g != nil
	if g == nil {
		if len(s.guests) >= 1800 {
			s.mu.Unlock()
			p.push(map[string]any{"type": "error", "message": "牌桌正忙，请稍后再来"})
			return
		}
		p.token = randomID()
		g = &guest{Player: &Player{ID: randomID(), Name: "牌友" + fmt.Sprintf("%04d", randomN(10000))}}
		s.guests[p.token] = g
	} else {
		p.token = hello.Token
	}
	if g.Player.Conn != nil {
		g.Player.Conn.close()
	}
	g.Player.Conn = p
	g.Seen = time.Now()
	p.push(map[string]any{"type": "welcome", "token": p.token, "name": g.Player.Name, "resumed": resumed})
	if room := s.rooms[g.Room]; room != nil {
		s.changed(room, true)
	} else {
		g.Room = ""
		p.push(map[string]any{"type": "state", "game": nil})
	}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if g.Player.Conn != p {
			return
		}
		g.Player.Conn = nil
		g.Seen = time.Now()
		if room := s.rooms[g.Room]; room != nil {
			s.changed(room, true)
		}
	}()
	_ = conn.SetReadDeadline(time.Now().Add(65 * time.Second))
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(65 * time.Second)) })
	window := time.Now()
	count := 0
	for {
		var a request
		if conn.ReadJSON(&a) != nil {
			return
		}
		if time.Since(window) > time.Second {
			window = time.Now()
			count = 0
		}
		count++
		if count > 20 {
			return
		}
		s.mu.Lock()
		if g.Player.Conn != p {
			s.mu.Unlock()
			return
		}
		g.Seen = time.Now()
		s.act(g, p, a)
		s.mu.Unlock()
	}
}
func (s *Server) failure(p *peer, message string) {
	p.push(map[string]any{"type": "error", "message": message})
}
func (s *Server) act(g *guest, p *peer, a request) {
	r := s.rooms[g.Room]
	if a.Type == "ping" {
		p.push(map[string]any{"type": "pong"})
		return
	}
	if a.Type == "leave" {
		s.leave(g)
		p.push(map[string]any{"type": "state", "game": nil})
		return
	}
	if a.Type == "practice" || a.Type == "create" || a.Type == "join" {
		if r != nil {
			s.failure(p, "请先离开当前房间")
			return
		}
		if a.Type == "join" {
			code := strings.ToUpper(strings.TrimSpace(a.Code))
			r = s.rooms[code]
			if r == nil || r.Practice {
				s.failure(p, "没有找到这个好友房，请核对房间码")
				return
			}
			if r.Phase != "waiting" || len(r.Players) >= 3 {
				s.failure(p, "房间已满或已经开局")
				return
			}
		} else {
			if len(s.rooms) >= 500 {
				s.failure(p, "牌桌正忙，请稍后重试")
				return
			}
			code := ""
			for code == "" || s.rooms[code] != nil {
				code = ""
				for range 6 {
					const alphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"
					code += string(alphabet[randomN(len(alphabet))])
				}
			}
			r = newRoom(code, a.Type == "practice")
			s.rooms[code] = r
		}
		g.Player.Hand = nil
		g.Player.Ready = false
		g.Player.Auto = false
		g.Player.Last = nil
		g.Player.Say = ""
		g.Player.Score = 0
		r.Players = append(r.Players, g.Player)
		g.Room = r.Code
		if r.Practice {
			r.Players = append(r.Players, &Player{ID: "bot:" + randomID(), Name: "晚晴", Bot: true}, &Player{ID: "bot:" + randomID(), Name: "松风", Bot: true})
			r.deal()
		}
		s.changed(r)
		return
	}
	if r == nil {
		s.failure(p, "请先进入牌桌")
		return
	}
	seat := r.seat(g.Player.ID)
	if seat < 0 {
		return
	}
	if a.Type == "ready" {
		if r.Phase != "waiting" && r.Phase != "ended" {
			return
		}
		g.Player.Ready = !g.Player.Ready
		ready := len(r.Players) == 3
		for _, q := range r.Players {
			ready = ready && (q.Ready || q.Bot)
		}
		if ready {
			r.deal()
		}
		s.changed(r)
		return
	}
	if a.Type == "auto" {
		if r.Phase != "playing" && r.Phase != "bidding" {
			return
		}
		g.Player.Auto = a.Auto
		s.changed(r, seat != r.Turn)
		return
	}
	if a.Version != r.Version {
		s.failure(p, "牌局已更新，请按当前回合操作")
		p.push(map[string]any{"type": "state", "game": r.view(g.Player.ID)})
		return
	}
	if seat != r.Turn || g.Player.Auto {
		s.failure(p, "请等轮到你出牌，或先取消托管")
		return
	}
	if a.Type == "hint" {
		if r.Phase != "playing" {
			return
		}
		moves := legalMoves(g.Player.Hand, r.Target)
		if len(moves) == 0 {
			p.push(map[string]any{"type": "hint", "version": r.Version, "cards": []int{}, "message": "没有能压过的牌，可以选择不出"})
			return
		}
		i := max(a.Index, 0) % len(moves)
		p.push(map[string]any{"type": "hint", "version": r.Version, "cards": sorted(moves[i]), "message": typeName(moves[i]) + " · 再点提示可换一组"})
		return
	}
	if a.Type == "bid" && r.Phase == "bidding" {
		r.bid(a.Bid)
		s.changed(r)
		return
	}
	if (a.Type == "play" || a.Type == "pass") && r.Phase == "playing" {
		if a.Type == "pass" {
			a.Cards = nil
		} else if len(a.Cards) == 0 {
			s.failure(p, "先点选要出的手牌")
			return
		}
		if err := r.play(a.Cards); err != nil {
			s.failure(p, err.Error())
			return
		}
		s.changed(r)
		return
	}
	s.failure(p, "当前阶段不能执行这个操作")
}
func (s *Server) changed(r *Room, preserve ...bool) {
	previousDeadline := r.Deadline
	r.Version++
	r.Touched = time.Now()
	if r.Timer != nil {
		r.Timer.Stop()
		r.Timer = nil
	}
	r.Deadline = time.Time{}
	if r.Phase == "bidding" || r.Phase == "playing" {
		p := r.Players[r.Turn]
		delay := 30 * time.Second
		if r.Phase == "bidding" {
			delay = 20 * time.Second
		}
		if p.Bot || p.Auto {
			delay = s.botDelay
		} else if r.Practice && p.Conn != nil {
			delay = 0
		}
		if len(preserve) > 0 && preserve[0] && !previousDeadline.IsZero() {
			delay = max(50*time.Millisecond, time.Until(previousDeadline))
		}
		if delay > 0 {
			r.Deadline = time.Now().Add(delay)
			version := r.Version
			r.Timer = time.AfterFunc(delay, func() { s.automatic(r, version) })
		}
	}
	for _, p := range r.Players {
		if p.Conn != nil {
			p.Conn.push(map[string]any{"type": "state", "game": r.view(p.ID)})
		}
	}
}
func (s *Server) automatic(r *Room, version int) {
	s.mu.Lock()
	if s.rooms[r.Code] != r || r.Version != version {
		s.mu.Unlock()
		return
	}
	seat := r.Turn
	p := r.Players[seat]
	phase := r.Phase
	hand := cards(p.Hand)
	var gctx bot.GameContext
	if phase == "playing" {
		gctx = r.botContext()
	}
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	bid := false
	var move []int
	if phase == "bidding" {
		bid = s.engine.DecideBid(ctx, p.Name, hand, nil)
	} else if phase == "playing" {
		move = cardIDs(s.engine.DecidePlay(ctx, p.Name, gctx))
	} else {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rooms[r.Code] != r || r.Version != version {
		return
	}
	if phase == "bidding" {
		r.bid(bid)
	} else {
		if err := r.play(move); err != nil {
			moves := legalMoves(p.Hand, r.Target)
			if len(moves) > 0 {
				_ = r.play(moves[0])
			} else {
				_ = r.play(nil)
			}
		}
	}
	s.changed(r)
}
func (s *Server) leave(g *guest) {
	r := s.rooms[g.Room]
	g.Room = ""
	if r == nil {
		return
	}
	seat := r.seat(g.Player.ID)
	if seat < 0 {
		return
	}
	if r.Practice {
		s.remove(r)
		return
	}
	if r.Phase == "bidding" || r.Phase == "playing" {
		replacement := *g.Player
		replacement.ID = "bot:" + randomID()
		replacement.Name = "电脑代打"
		replacement.Bot = true
		replacement.Auto = false
		replacement.Conn = nil
		r.Players[seat] = &replacement
		r.event(seat, "已离桌，由电脑代打", nil)
	} else {
		r.Players = slices.Delete(r.Players, seat, seat+1)
		r.Phase = "waiting"
		r.Landlord = -1
		r.Winner = -1
		r.Bottom = nil
		r.Target = rule.ParsedHand{}
		r.History = nil
		for _, p := range r.Players {
			p.Hand = nil
			p.Last = nil
			p.Say = ""
			p.Ready = false
		}
	}
	human := false
	for _, p := range r.Players {
		human = human || !p.Bot
	}
	if !human {
		s.remove(r)
	} else {
		s.changed(r)
	}
}
func (s *Server) remove(r *Room) {
	if r.Timer != nil {
		r.Timer.Stop()
	}
	delete(s.rooms, r.Code)
	for _, g := range s.guests {
		if g.Room == r.Code {
			g.Room = ""
			if g.Player.Conn != nil {
				g.Player.Conn.push(map[string]any{"type": "state", "game": nil, "message": "房间已结束或长时间无人操作"})
			}
		}
	}
}
func (s *Server) cleanupLocked() {
	now := time.Now()
	for _, r := range s.rooms {
		online := false
		lastSeen := time.Time{}
		for _, g := range s.guests {
			if g.Room == r.Code {
				online = online || g.Player.Conn != nil
				if g.Seen.After(lastSeen) {
					lastSeen = g.Seen
				}
			}
		}
		if (!online && now.Sub(lastSeen) > 2*time.Minute) || ((r.Phase == "waiting" || r.Phase == "ended") && now.Sub(r.Touched) > 30*time.Minute) {
			s.remove(r)
		}
	}
	for token, g := range s.guests {
		if g.Player.Conn == nil && g.Room == "" && now.Sub(g.Seen) > 30*time.Minute {
			delete(s.guests, token)
		}
	}
}
func (s *Server) RunCleanup(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.mu.Lock()
			for _, r := range s.rooms {
				s.remove(r)
			}
			for _, g := range s.guests {
				if g.Player.Conn != nil {
					g.Player.Conn.close()
				}
			}
			s.mu.Unlock()
			return
		case <-ticker.C:
			s.mu.Lock()
			s.cleanupLocked()
			log.Printf("webgame active rooms=%d guests=%d", len(s.rooms), len(s.guests))
			s.mu.Unlock()
		}
	}
}
