package main

import (
	"github.com/thoj/go-ircevent"
	"strings"
	"fmt"
)

type handler func(conn *irc.Connection) 
type matcherFn func(man *manager, e *irc.Event) handler
type matcher struct {
	id string
	mFn matcherFn
}
type manager struct {
	matchers map[string]matcher
}

func (man *manager) remove(id string) {
	if _, ok := man.matchers[id]; ok {
		delete(man.matchers, id)
	}
}

func (man *manager) add(m matcher) {
	man.matchers[m.id] = m
}

func NewManager() *manager {
	man := &manager{make(map[string]matcher, 4)}
	
	man.add(matcher{"nsauth", matchNickservAuth})
	
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
	
	man := NewManager()
	
	matchAndHandle := func(e *irc.Event) {
		for _, m := range man.matchers {
			h := m.mFn(man, e)
			if h != nil {
				h(conn)
			}
		}
	}
		
	conn.AddCallback("001", func(e *irc.Event) { conn.Join("#squishyslab") })
	
	conn.AddCallback("PRIVMSG", matchAndHandle)
	conn.AddCallback("NOTICE", matchAndHandle)
	
	conn.Loop()
}

func matchNickservAuth(man *manager, e *irc.Event) handler {
	if strings.Contains(strings.ToLower(e.Message()), "identify") && e.User == "NickServ" {
		man.remove("nsauth")
		
		return authNickserv
	}
	
	return nil
}

func authNickserv(conn *irc.Connection) {
	conn.Privmsg("NickServ", "IDENTIFY user password")
}