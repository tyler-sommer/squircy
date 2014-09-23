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
	Handle(man *Manager, e *irc.Event)
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
	man.Add(&AliasHandler{make(map[string]string, 4)})

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
				h.Handle(man, e)
			}
		}
	}

	conn.AddCallback("001", func(e *irc.Event) { conn.Join(config.Channel) })

	conn.AddCallback("PRIVMSG", matchAndHandle)
	conn.AddCallback("NOTICE", matchAndHandle)

	conn.Loop()
}

func ReplyTarget(e *irc.Event) string {
	if strings.HasPrefix(e.Arguments[0], "#") {
		return e.Arguments[0]
	} else {
		return e.Nick
	}
}

type NickservAuth struct{}

func (h *NickservAuth) Id() string {
	return "nsauth"
}

func (h *NickservAuth) Matches(e *irc.Event) bool {
	return strings.Contains(strings.ToLower(e.Message()), "identify") && e.User == "NickServ"
}

func (h *NickservAuth) Handle(man *Manager, e *irc.Event) {
	man.Remove(h)
	man.conn.Privmsgf("NickServ", "IDENTIFY %s", man.config.Password)
}

type AliasHandler struct{
	aliases map[string]string
}

func (h *AliasHandler) Id() string {
	return "alias"
}

func (h *AliasHandler) Matches(e *irc.Event) bool {
	return strings.HasPrefix(strings.ToLower(e.Message()), "!")
}

func (h *AliasHandler) Handle(man *Manager, e *irc.Event) {
	fields := strings.Fields(e.Message())
	command := fields[0][1:]
	message, ok := h.aliases[command]
	switch {
	case ok:
		man.conn.Privmsgf(ReplyTarget(e), message)
		
		break;
	case fields[0] != "!alias":
		break
		
		
	case fields[1] == "add":
		man.conn.Privmsgf(ReplyTarget(e), "Adding %s", fields[2:])
		h.aliases[fields[2]] = strings.Join(fields[3:], " ")

		break
	}
}
