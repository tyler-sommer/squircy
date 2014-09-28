package main

import (
	"./squircy"
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/thoj/go-ircevent"
	"os"
	"sync"
	"time"
)

func main() {
	file, err := os.Open("config.json")
	if err != nil {
		panic("Could not open config.json: " + err.Error())
	}

	decoder := json.NewDecoder(file)

	config := squircy.Configuration{}
	if err := decoder.Decode(&config); err != nil {
		panic("Could not decode config.json: " + err.Error())
	}

	conn := irc.IRC(config.Nick, config.Username)
	conn.Debug = true

	err = conn.Connect(config.Network)
	if err != nil {
		panic(err)
	}

	man := squircy.NewManager(conn, config)

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

	go conn.Loop()

	bin := bufio.NewReader(os.Stdin)
	for {
		switch str, _ := bin.ReadString('\n'); {
		case str == "exit\n" || str == "quit\n":
			conn.Quit()
			time.Sleep(2 * time.Second)
			fmt.Println("Exiting")
			return

		case str == "debug\n":
			was := conn.VerboseCallbackHandler
			conn.VerboseCallbackHandler = !was
			if was {
				fmt.Println("Debug DISABLED")
			} else {
				fmt.Println("Debug ENABLED")
			}

		default:
			fmt.Println(`Unknown input. Commands:

exit		Quits IRC and exits the program
debug		Toggles debug
`)
		}
	}
}
