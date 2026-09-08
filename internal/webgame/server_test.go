package webgame

import (
	"context"
	"encoding/json"
	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

type wire struct {
	Type    string `json:"type"`
	Token   string `json:"token"`
	Game    *View  `json:"game"`
	Message string `json:"message"`
}

func testServer(t *testing.T) (*Server, string) {
	t.Helper()
	s := New([]string{"https://test.invalid"}, NewFallback(), false)
	ctx, cancel := context.WithCancel(context.Background())
	go s.RunCleanup(ctx)
	srv := httptest.NewServer(http.HandlerFunc(s.socket))
	t.Cleanup(func() { cancel(); srv.Close() })
	return s, "ws" + strings.TrimPrefix(srv.URL, "http")
}
func joinSocket(t *testing.T, url, token string) (*websocket.Conn, string) {
	t.Helper()
	c, _, e := websocket.DefaultDialer.Dial(url, http.Header{"Origin": []string{"https://test.invalid"}})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { c.Close() })
	if e = c.WriteJSON(request{Type: "hello", Token: token}); e != nil {
		t.Fatal(e)
	}
	w := readWire(t, c, func(w wire) bool { return w.Type == "welcome" })
	return c, w.Token
}
func readWire(t *testing.T, c *websocket.Conn, match func(wire) bool) wire {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		_, b, e := c.ReadMessage()
		if e != nil {
			t.Fatal(e)
		}
		var w wire
		if e = json.Unmarshal(b, &w); e != nil {
			t.Fatal(e)
		}
		if match(w) {
			return w
		}
	}
}
func readGame(t *testing.T, c *websocket.Conn, match func(*View) bool) *View {
	return readWire(t, c, func(w wire) bool { return w.Type == "state" && w.Game != nil && match(w.Game) }).Game
}
func TestFriendRoomReadyReconnectAndStaleMoves(t *testing.T) {
	s, url := testServer(t)
	var cs [3]*websocket.Conn
	var tokens [3]string
	for i := range cs {
		cs[i], tokens[i] = joinSocket(t, url, "")
	}
	_ = cs[0].WriteJSON(request{Type: "create"})
	v := readGame(t, cs[0], func(v *View) bool { return len(v.Players) == 1 })
	for i := 1; i < 3; i++ {
		_ = cs[i].WriteJSON(request{Type: "join", Code: v.Code})
		readGame(t, cs[i], func(v *View) bool { return len(v.Players) == i+1 })
	}
	for _, c := range cs {
		_ = c.WriteJSON(request{Type: "ready"})
	}
	for _, c := range cs {
		v = readGame(t, c, func(v *View) bool { return v.Phase == "bidding" })
	}
	version := v.Version
	// A newly attached socket replaces the transport, not the player's identity.
	oldTurn := v.Turn
	oldHand := safelyView(s, v.Code, v.Players[oldTurn].ID).Hand
	cs[oldTurn].Close()
	replacement, _ := joinSocket(t, url, tokens[oldTurn])
	cs[oldTurn] = replacement
	v = readGame(t, replacement, func(v *View) bool { return v.Phase == "bidding" })
	if v.You != oldTurn || !reflect.DeepEqual(v.Hand, oldHand) || len(v.Bottom) != 0 {
		t.Fatal("reconnect identity/hand/privacy failure")
	}
	for _, p := range v.Players {
		if len(p.Hand) > 0 {
			t.Fatal("opponent hand leak")
		}
	}
	_ = replacement.WriteJSON(request{Type: "bid", Bid: true, Version: version})
	readWire(t, replacement, func(w wire) bool { return w.Type == "error" })
	v = readGame(t, replacement, func(v *View) bool { return v.Version > version })
	for i := 0; i < 3; i++ {
		seat := v.Turn
		_ = cs[seat].WriteJSON(request{Type: "bid", Bid: i == 0, Version: v.Version})
		old := v.Version
		v = readGame(t, cs[seat], func(n *View) bool { return n.Version > old })
	}
	if v.Phase != "playing" || v.Landlord != oldTurn {
		t.Fatal("calling/grabbing failed")
	}
	// Duplicate submissions carry the old revision and cannot remove two moves.
	hand := safelyView(s, v.Code, v.Players[v.Turn].ID).Hand
	actor := cs[v.Turn]
	request := request{Type: "play", Cards: hand[:1], Version: v.Version}
	_ = actor.WriteJSON(request)
	_ = actor.WriteJSON(request)
	after := readGame(t, actor, func(n *View) bool { return n.Version > v.Version })
	readWire(t, actor, func(w wire) bool { return w.Type == "error" })
	if len(after.Hand) != len(hand)-1 {
		t.Fatal("duplicate move changed hand twice")
	}
	// Leaving an active friend game explicitly replaces only that seat with a bot.
	_ = actor.WriteJSON(struct {
		Type string `json:"type"`
	}{"leave"})
	readWire(t, actor, func(w wire) bool { return w.Type == "state" && w.Game == nil })
	s.mu.Lock()
	r := s.rooms[v.Code]
	replaced := r.Players[oldTurn].Bot
	s.mu.Unlock()
	if !replaced {
		t.Fatal("leaver not replaced")
	}
}
func safelyView(s *Server, code, id string) View {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rooms[code].view(id)
}
func TestOriginAndUnknownRoom(t *testing.T) {
	_, url := testServer(t)
	c, response, e := websocket.DefaultDialer.Dial(url, http.Header{"Origin": []string{"https://evil.invalid"}})
	if c != nil {
		c.Close()
	}
	if e == nil || response.StatusCode != 403 {
		t.Fatal("foreign origin accepted")
	}
	c, _ = joinSocket(t, url, "")
	_ = c.WriteJSON(request{Type: "join", Code: "ZZZZZZ"})
	w := readWire(t, c, func(w wire) bool { return w.Type == "error" })
	if !strings.Contains(w.Message, "没有找到") {
		t.Fatal(w)
	}
}
