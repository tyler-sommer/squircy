package main

import (
	"encoding/json"
	"fmt"
	"github.com/thoj/go-ircevent"
	"os"
	"strings"
)

type Configuration struct {
	Network  string
	Nick     string
	Username string
	Password string
	Channel  string
}

type Handler interface {
	Id() string
	Matches(e *irc.Event) bool
	Handle(man *Manager)
}

type Manager struct {
	conn     *irc.Connection
	config   Configuration
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

func NewManager(conn *irc.Connection, config Configuration) *Manager {
	man := &Manager{conn, config, make(map[string]Handler, 4)}

	man.Add(&NickservAuth{})

	return man
}

func main() {
	file, err := os.Open("config.json")
	if err != nil {
		panic("Could not open config.json: " + err.Error())
	}

	decoder := json.NewDecoder(file)

	config := Configuration{}
	if err := decoder.Decode(&config); err != nil {
		panic("Could not decode config.json: " + err.Error())
	}

	conn := irc.IRC(config.Nick, config.Username)
	conn.Debug = true
	conn.VerboseCallbackHandler = true

	err = conn.Connect(config.Network)
	if err != nil {
		panic(err)
	}

	man := NewManager(conn, config)

	matchAndHandle := func(e *irc.Event) {
		for _, h := range man.handlers {
			if h.Matches(e) {
				h.Handle(man)
			}
		}
	}

	conn.AddCallback("001", func(e *irc.Event) { conn.Join(config.Channel) })

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
	man.Remove(h)
	man.conn.Privmsgf("NickServ", "IDENTIFY %s", man.config.Password)
}
