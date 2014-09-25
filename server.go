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
	Handle(e *irc.Event)
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

	man.Add(newNickservHandler(man))
	man.Add(newAliasHandler(man))
	man.Add(newJavascriptHandler(man))
	man.Add(newLuaHandler(man))
	man.Add(newLispHandler(man))

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
				h.Handle(e)
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

func newNickservHandler(man *Manager) *NickservHandler {
	return &NickservHandler{man}
}

type NickservHandler struct{
	man *Manager
}

func (h *NickservHandler) Id() string {
	return "nickserv"
}

func (h *NickservHandler) Matches(e *irc.Event) bool {
	return strings.Contains(strings.ToLower(e.Message()), "identify") && e.User == "NickServ"
}

func (h *NickservHandler) Handle(e *irc.Event) {
	h.man.Remove(h)
	h.man.conn.Privmsgf("NickServ", "IDENTIFY %s", h.man.config.Password)
}

func newAliasHandler(man *Manager) *AliasHandler {
	return &AliasHandler{man, make(map[string]string, 4)}
}

type AliasHandler struct {
	man *Manager
	aliases map[string]string
}

func (h *AliasHandler) Id() string {
	return "alias"
}

func (h *AliasHandler) Matches(e *irc.Event) bool {
	return strings.HasPrefix(strings.ToLower(e.Message()), "!")
}

func (h *AliasHandler) Handle(e *irc.Event) {
	command, args := parseCommand(e.Message())

	message, ok := h.aliases[command]
	switch {
	case command == "alias":
		if len(args) < 2 {
			h.man.conn.Privmsgf(replyTarget(e), "Usage: !alias <add/remove> name [message]")

		} else if args[0] == "add" {
			h.aliases[args[1]] = strings.Join(args[2:], " ")
			h.man.conn.Privmsgf(replyTarget(e), "Added '%s'", args[1])

		} else if args[0] == "remove" {
			if _, ok := h.aliases[args[1]]; ok {
				delete(h.aliases, args[1])
				h.man.conn.Privmsgf(replyTarget(e), "Removed '%s'", args[1])
			}
		}

	case ok:
		h.man.conn.Privmsgf(replyTarget(e), message)
	}
}

func newJavascriptHandler(man *Manager) *JavascriptHandler {
	return &JavascriptHandler{man, otto.New()}
}

type JavascriptHandler struct {
	man *Manager
	vm *otto.Otto
}

func (h *JavascriptHandler) Id() string {
	return "js"
}

func (h *JavascriptHandler) Matches(e *irc.Event) bool {
	return strings.HasPrefix(strings.ToLower(e.Message()), "!js") && e.Nick == h.man.config.OwnerNick && e.Host == h.man.config.OwnerHost
}

func (h *JavascriptHandler) Handle(e *irc.Event) {
	fields := strings.Fields(e.Message())

	value, err := h.vm.Run(strings.Join(fields[1:], " "))
	if err != nil {
		h.man.conn.Privmsgf(replyTarget(e), err.Error())

		return
	}
	h.man.conn.Privmsgf(replyTarget(e), value.String())
}

func newLuaHandler(man *Manager) *LuaHandler {
	return &LuaHandler{man, lua.NewState()}
}

type LuaHandler struct {
	man *Manager
	vm *lua.State
}

func (h *LuaHandler) Id() string {
	return "lua"
}

func (h *LuaHandler) Matches(e *irc.Event) bool {
	return strings.HasPrefix(strings.ToLower(e.Message()), "!lua") && e.Nick == h.man.config.OwnerNick && e.Host == h.man.config.OwnerHost
}

func (h *LuaHandler) Handle(e *irc.Event) {
	fields := strings.Fields(e.Message())
	printFn := func(vm *lua.State) int {
		o := vm.ToString(1)
		h.man.conn.Privmsgf(replyTarget(e), o)
		return 0
	}
	h.vm.Register("print", printFn)
	err := h.vm.DoString(strings.Join(fields[1:], " "))
	if err != nil {
		h.man.conn.Privmsgf(replyTarget(e), err.Error())
	}
}

func newLispHandler(man *Manager) *LispHandler {
	return &LispHandler{man}
}

type LispHandler struct{
	man *Manager
}

func (h *LispHandler) Id() string {
	return "lisp"
}

func (h *LispHandler) Matches(e *irc.Event) bool {
	return strings.HasPrefix(strings.ToLower(e.Message()), "!lisp") && e.Nick == h.man.config.OwnerNick && e.Host == h.man.config.OwnerHost
}

func (h *LispHandler) Handle(e *irc.Event) {
	fields := strings.Fields(e.Message())
	val, err := lisp.EvalString(strings.Join(fields[1:], " "))
	if err != nil {
		h.man.conn.Privmsgf(replyTarget(e), err.Error())

		return
	}
	h.man.conn.Privmsgf(replyTarget(e), val.String())
}

		return
	}
	h.man.conn.Privmsgf(replyTarget(e), value.String())
}
