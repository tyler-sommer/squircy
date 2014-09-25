package squircy

import (
	"github.com/thoj/go-ircevent"
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

type Handlers map[string]Handler

type Manager struct {
	conn     *irc.Connection
	config   Configuration
	handlers Handlers
}
