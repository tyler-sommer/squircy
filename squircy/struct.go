package squircy

import (
	"github.com/thoj/go-ircevent"
	"github.com/fzzy/radix/redis"
	"github.com/go-martini/martini"
)

type Configuration struct {
	Network       string
	Nick          string
	Username      string
	Password      string
	Channel       string
	OwnerNick     string
	OwnerHost     string
	RedisHost     string
	RedisDatabase int
}

type Handler interface {
	Id() string
	Matches(e *irc.Event) bool
	Handle(e *irc.Event)
}

type Handlers map[string]Handler

type Manager struct {
	conn     *irc.Connection
	m		 *martini.ClassicMartini
	c		 *redis.Client
	config   Configuration
	handlers Handlers
}
