package squircy

import (
	"fmt"
	"github.com/thoj/go-ircevent"
)

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

func (man *Manager) Handlers() *Handlers {
	return &man.handlers
}

func NewManager(conn *irc.Connection, config Configuration) *Manager {
	man := &Manager{conn, config, make(map[string]Handler, 4)}

	man.Add(newNickservHandler(man))
	man.Add(newAliasHandler(man))
	man.Add(newJavascriptHandler(man))
	man.Add(newLuaHandler(man))
	man.Add(newLispHandler(man))
	man.Add(newScriptHandler(man))

	return man
}
