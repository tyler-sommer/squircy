package main

import (
	"encoding/json"
	"fmt"
	"github.com/aarzilli/golua/lua"
	"github.com/janne/go-lisp/lisp"
	"github.com/robertkrimen/otto"
	"github.com/thoj/go-ircevent"
	"os"
	"strings"
	"sync"
)

type Configuration struct {
	Network   string
	Nick      string
	Username  string
	Password  string
	Channel   string
	OwnerNick string
	OwnerHost string
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
	if _, ok := man.handlers[h.Id()]; ok {
		fmt.Println("Removing handler ", h.Id())
		delete(man.handlers, h.Id())
	}
}

func (man *Manager) RemoveId(id string) {
	if handler, ok := man.handlers[id]; ok {
		man.Remove(handler)
	}
}

func (man *Manager) Add(h Handler) {
	fmt.Println("Adding handler ", h.Id())
	man.handlers[h.Id()] = h
}

func NewManager(conn *irc.Connection, config Configuration) *Manager {
	man := &Manager{conn, config, make(map[string]Handler, 4)}

	man.Add(&NickservHandler{})
	man.Add(&AliasHandler{make(map[string]string, 4)})
	man.Add(newJavascriptHandler())
	man.Add(newLuaHandler())
	man.Add(newLispHandler())

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

	mutex := &sync.Mutex{}
	matchAndHandle := func(e *irc.Event) {
		mutex.Lock()
		for _, h := range man.handlers {
			if h.Matches(e) {
				h.Handle(man, e)
			}
		}
		mutex.Unlock()
	}

	conn.AddCallback("001", func(e *irc.Event) { conn.Join(config.Channel) })

	conn.AddCallback("PRIVMSG", matchAndHandle)
	conn.AddCallback("NOTICE", matchAndHandle)

	conn.Loop()
}

func replyTarget(e *irc.Event) string {
	if strings.HasPrefix(e.Arguments[0], "#") {
		return e.Arguments[0]
	} else {
		return e.Nick
	}
}

func parseCommand(msg string) (string, []string) {
	fields := strings.Fields(msg)
	if len(fields) < 1 {
		return "", nil
	}

	command := fields[0][1:]
	args := fields[1:]

	return command, args
}

type NickservHandler struct{}

func (h *NickservHandler) Id() string {
	return "nickserv"
}

func (h *NickservHandler) Matches(e *irc.Event) bool {
	return strings.Contains(strings.ToLower(e.Message()), "identify") && e.User == "NickServ"
}

func (h *NickservHandler) Handle(man *Manager, e *irc.Event) {
	man.Remove(h)
	man.conn.Privmsgf("NickServ", "IDENTIFY %s", man.config.Password)
}

type AliasHandler struct {
	aliases map[string]string
}

func (h *AliasHandler) Id() string {
	return "alias"
}

func (h *AliasHandler) Matches(e *irc.Event) bool {
	return strings.HasPrefix(strings.ToLower(e.Message()), "!")
}

func (h *AliasHandler) Handle(man *Manager, e *irc.Event) {
	command, args := parseCommand(e.Message())

	message, ok := h.aliases[command]
	switch {
	case command == "alias":
		if len(args) < 2 {
			man.conn.Privmsgf(replyTarget(e), "Usage: !alias <add/remove> name [message]")

		} else if args[0] == "add" {
			h.aliases[args[1]] = strings.Join(args[2:], " ")
			man.conn.Privmsgf(replyTarget(e), "Added '%s'", args[1])

		} else if args[0] == "remove" {
			if _, ok := h.aliases[args[1]]; ok {
				delete(h.aliases, args[1])
				man.conn.Privmsgf(replyTarget(e), "Removed '%s'", args[1])
			}
		}

	case ok:
		man.conn.Privmsgf(replyTarget(e), message)
	}
}

func newJavascriptHandler() *JavascriptHandler {
	return &JavascriptHandler{otto.New()}
}

type JavascriptHandler struct {
	vm *otto.Otto
}

func (h *JavascriptHandler) Id() string {
	return "js"
}

func (h *JavascriptHandler) Matches(e *irc.Event) bool {
	return strings.HasPrefix(strings.ToLower(e.Message()), "!js")
}

func (h *JavascriptHandler) Handle(man *Manager, e *irc.Event) {
	fields := strings.Fields(e.Message())

	value, err := h.vm.Run(strings.Join(fields[1:], " "))
	if err != nil {
		man.conn.Privmsgf(replyTarget(e), err.Error())

		return
	}
	man.conn.Privmsgf(replyTarget(e), value.String())
}

func newLuaHandler() *LuaHandler {
	return &LuaHandler{lua.NewState()}
}

type LuaHandler struct {
	vm *lua.State
}

func (h *LuaHandler) Id() string {
	return "lua"
}

func (h *LuaHandler) Matches(e *irc.Event) bool {
	return strings.HasPrefix(strings.ToLower(e.Message()), "!lua")
}

func (h *LuaHandler) Handle(man *Manager, e *irc.Event) {
	fields := strings.Fields(e.Message())
	printFn := func(vm *lua.State) int {
		o := vm.ToString(1)
		man.conn.Privmsgf(replyTarget(e), o)
		return 0
	}
	h.vm.Register("print", printFn)
	err := h.vm.DoString(strings.Join(fields[1:], " "))
	if err != nil {
		man.conn.Privmsgf(replyTarget(e), err.Error())
	}
}

func newLispHandler() *LispHandler {
	return &LispHandler{}
}

type LispHandler struct{}

func (h *LispHandler) Id() string {
	return "lisp"
}

func (h *LispHandler) Matches(e *irc.Event) bool {
	return strings.HasPrefix(strings.ToLower(e.Message()), "!lisp")
}

func (h *LispHandler) Handle(man *Manager, e *irc.Event) {
	fields := strings.Fields(e.Message())
	val, err := lisp.EvalString(strings.Join(fields[1:], " "))
	if err != nil {
		man.conn.Privmsgf(replyTarget(e), err.Error())

		return
	}
	man.conn.Privmsgf(replyTarget(e), val.String())
}
