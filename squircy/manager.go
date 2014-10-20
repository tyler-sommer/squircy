package squircy

import (
	"fmt"
	"github.com/thoj/go-ircevent"
	"github.com/fzzy/radix/redis"
	"github.com/go-martini/martini"
	"sync"
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

func (man *Manager) Quit() {
	man.conn.Quit()
}

func (man *Manager) Debug(enabled bool) {
	man.conn.VerboseCallbackHandler = enabled
	man.conn.Debug = enabled
}

func (man *Manager) DebugEnabled() bool {
	return man.conn.VerboseCallbackHandler
}

func NewManager(config Configuration) *Manager {
	conn := newIrcConnection(config)
	r := newRedisClient(config)
	m := newMartiniClassic(config)
	
	man := &Manager{conn, m, r, config, make(map[string]Handler, 4)}
	
	mutex := &sync.Mutex{}
	matchAndHandle := func(e *irc.Event) {
		mutex.Lock()
		for _, h := range *man.Handlers() {
			if h.Matches(e) {
				h.Handle(e)
			}
		}
		mutex.Unlock()
	}

	conn.AddCallback("001", func(e *irc.Event) { conn.Join(config.Channel) })

	conn.AddCallback("PRIVMSG", matchAndHandle)
	conn.AddCallback("NOTICE", matchAndHandle)

	man.Add(newNickservHandler(man))
	man.Add(newAliasHandler(man))
	man.Add(newScriptHandler(man))

	return man
}

func newIrcConnection(config Configuration) (conn *irc.Connection) {
	conn = irc.IRC(config.Nick, config.Username)
	conn.Debug = true

	err := conn.Connect(config.Network)
	if err != nil {
		panic(err)
	}
	
	go conn.Loop()
	
	return
}

func newRedisClient(config Configuration) (c *redis.Client) {
	c, err := redis.Dial("tcp", config.RedisHost)
	if err != nil {
		panic("Error connecting to redis")
	}
	
	r := c.Cmd("select", config.RedisDatabase)
	if r.Err != nil {
		panic("Error selecting redis database")
	}
	
	return
}

func newMartiniClassic(config Configuration) (m *martini.ClassicMartini) {
	m = martini.Classic()
	m.Get("/", func() string {
		return "Hello, world!"
	})
	
	go m.Run()
	
	return
}