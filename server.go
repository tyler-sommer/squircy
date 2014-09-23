package main

import (
	"fmt"
	"github.com/thoj/go-ircevent"
	"strings"
)

type Handler interface {
	Id() string
	Matches(e *irc.Event) bool
	Handle(man *Manager)
}

type Manager struct {
	conn     *irc.Connection
	handlers map[string]Handler
}

func (man *Manager) Remove(h Handler) {
	fmt.Println("Removing handler ", h.Id())
	if _, ok := man.handlers[h.Id()]; ok {
		delete(man.handlers, h.Id())
	}
}

func (man *Manager) Add(h Handler) {
	fmt.Println("Adding handler ", h.Id())
	man.handlers[h.Id()] = h
}

func NewManager(conn *irc.Connection) *Manager {
	man := &Manager{conn, make(map[string]Handler, 4)}

	man.Add(&NickservAuth{})

	return man
}

func main() {
	conn := irc.IRC("squishyj", "squishyj")
	conn.Debug = true
	conn.VerboseCallbackHandler = true

	err := conn.Connect("irc.freenode.net:6667")
	if err != nil {
		panic(err)
	}

	man := NewManager(conn)

	matchAndHandle := func(e *irc.Event) {
		for _, h := range man.handlers {
			if h.Matches(e) {
				h.Handle(man)
			}
		}
	}

	conn.AddCallback("001", func(e *irc.Event) { conn.Join("#squishyslab") })

	conn.AddCallback("PRIVMSG", matchAndHandle)
	conn.AddCallback("NOTICE", matchAndHandle)

	conn.Loop()
}

type NickservAuth struct{}

func (h *NickservAuth) Id() string {
	return "nsauth"
}

func (h *NickservAuth) Matches(e *irc.Event) bool {
	return strings.Contains(strings.ToLower(e.Message()), "identify") && e.User == "NickServ"
}

func (h *NickservAuth) Handle(man *Manager) {
	man.conn.Privmsg("NickServ", "IDENTIFY user password")
}
