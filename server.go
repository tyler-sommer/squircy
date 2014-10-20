package main

import (
	"./squircy"
	"bufio"
	"encoding/json"
	"fmt"
	"os"
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
	
	man := squircy.NewManager(config)
	
	bin := bufio.NewReader(os.Stdin)
	for {
		switch str, _ := bin.ReadString('\n'); {
		case str == "exit\n" || str == "quit\n":
			man.Quit()
			time.Sleep(2 * time.Second)
			fmt.Println("Exiting")
			return

		case str == "debug\n":
			was := man.DebugEnabled()
			man.Debug(!was)
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
