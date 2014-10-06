package squircy

import (
	"errors"
	"fmt"
	"github.com/aarzilli/golua/lua"
	"github.com/robertkrimen/otto"
	"github.com/thoj/go-ircevent"
	"github.com/veonik/go-lisp/lisp"
	"strings"
	"time"
)

const maxExecutionTime = 2 // in seconds
var halt = errors.New("Execution limit exceeded")

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

type NickservHandler struct {
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
	man     *Manager
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

type ScriptDatastore map[string]string

type ScriptHandler struct {
	man      *Manager
	luaVm    *lua.State
	jsVm     *otto.Otto
	repl     bool
	replType string
	data     ScriptDatastore
}

func newScriptHandler(man *Manager) *ScriptHandler {
	luaVm := lua.NewState()
	luaVm.OpenLibs()

	jsVm := otto.New()

	return &ScriptHandler{man, luaVm, jsVm, false, "", make(ScriptDatastore)}
}

func (h *ScriptHandler) Id() string {
	return "scripting"
}

func (h *ScriptHandler) Matches(e *irc.Event) bool {
	return e.Nick == h.man.config.OwnerNick && e.Host == h.man.config.OwnerHost
}

func replTypePretty(replType string) string {
	switch {
	case replType == "lua":
		return "Lua"

	case replType == "js":
		return "Javascript"

	case replType == "lisp":
		return "Lisp"
	}

	return "Unknown"
}

func scriptRecoveryHandler(man *Manager, e *irc.Event) {
	if err := recover(); err != nil {
		fmt.Println("An error occurred", err)
		if err == halt {
			man.conn.Privmsgf(replyTarget(e), "Script halted")
		}
	}
}

func (h *ScriptHandler) Handle(e *irc.Event) {
	defer scriptRecoveryHandler(h.man, e)

	if h.repl == true {
		msg := e.Message()
		if strings.HasPrefix(msg, "!repl end") {
			h.man.conn.Privmsgf(replyTarget(e), "%s REPL session ended.", replTypePretty(h.replType))
			h.repl = false
			h.replType = ""
			return
		}

		switch {
		case h.replType == "lua":
			h.luaVm.Register("typename", func(vm *lua.State) int {
				o := vm.Typename(int(vm.Type(1)))
				h.luaVm.PushString(o)
				return 1
			})
			h.luaVm.Register("print", func(vm *lua.State) int {
				o := vm.ToString(1)
				h.man.conn.Privmsgf(replyTarget(e), o)
				return 0
			})
			h.luaVm.Register("setExternalProperty", func(vm *lua.State) int {
				key := vm.ToString(1)
				value := vm.ToString(2)
				h.data[key] = value
				return 0
			})
			h.luaVm.Register("getExternalProperty", func(vm *lua.State) int {
				key := vm.ToString(1)
				if val, ok := h.data[key]; ok {
					vm.PushString(val)
					return 1
				}
				return 0
			})
			err := runUnsafeLua(h.luaVm, msg)
			if err != nil {
				h.man.conn.Privmsgf(replyTarget(e), err.Error())
			}

		case h.replType == "js":
			h.jsVm.Set("setExternalProperty", func(call otto.FunctionCall) otto.Value {
				key, _ := call.Argument(0).ToString()
				value, _ := call.Argument(1).ToString()
				h.data[key] = value
				return otto.Value{}
			})
			h.jsVm.Set("getExternalProperty", func(call otto.FunctionCall) otto.Value {
				key, _ := call.Argument(0).ToString()
				if val, ok := h.data[key]; ok {
					result, _ := h.jsVm.ToValue(val)
					return result
				}
				return otto.Value{}
			})
			h.jsVm.Set("print", func(call otto.FunctionCall) otto.Value {
				message, _ := call.Argument(0).ToString()
				h.man.conn.Privmsgf(replyTarget(e), message)
				return otto.Value{}
			})
			_, err := runUnsafeJavascript(h.jsVm, msg)
			if err != nil {
				h.man.conn.Privmsgf(replyTarget(e), err.Error())

				return
			}

		case h.replType == "lisp":
			lisp.SetHandler("print", func(vars ...lisp.Value) (lisp.Value, error) {
				if len(vars) == 1 {
					h.man.conn.Privmsgf(replyTarget(e), vars[0].String())
				}
				return lisp.Nil, nil
			})
			lisp.SetHandler("setex", func(vars ...lisp.Value) (lisp.Value, error) {
				if len(vars) != 2 {
					return lisp.Nil, nil
				}
				key := vars[0].String()
				value := vars[1].String()
				h.data[key] = value
				return lisp.Nil, nil
			})
			lisp.SetHandler("getex", func(vars ...lisp.Value) (lisp.Value, error) {
				if len(vars) != 1 {
					return lisp.Nil, nil
				}
				key := vars[0].String()
				if val, ok := h.data[key]; ok {
					return lisp.StringValue(val), nil
				}
				return lisp.Nil, nil
			})
			_, err := runUnsafeLisp(msg)
			if err != nil {
				h.man.conn.Privmsgf(replyTarget(e), err.Error())

				return
			}
		}

		return
	}

	switch command, args := parseCommand(e.Message()); {
	case command == "":
		break

	case command == "register":
		if len(args) != 2 && args[0] != "js" && args[0] != "lua" && args[0] != "lisp" {
			h.man.conn.Privmsgf(replyTarget(e), "Invalid syntax. Usage: !register <js|lua|lisp> <fn name>")

			return
		}

		switch {
		case args[0] == "js":
			handler := newJavascriptScript(h.man, h.jsVm, args[1])
			h.man.Remove(handler)
			h.man.Add(handler)

		case args[0] == "lua":
			handler := newLuaScript(h.man, h.luaVm, args[1])
			h.man.Remove(handler)
			h.man.Add(handler)

		case args[0] == "lisp":
			handler := newLispScript(h.man, args[1])
			h.man.Remove(handler)
			h.man.Add(handler)
		}

	case command == "unregister":
		if len(args) != 2 && args[0] != "js" && args[0] != "lua" && args[0] != "lisp" {
			h.man.conn.Privmsgf(replyTarget(e), "Invalid syntax. Usage: !unregister <js|lua|lisp> <fn name>")

			return
		}

		switch {
		case args[0] == "js":
			h.man.conn.Privmsgf(replyTarget(e), "Unregistered Javsacript handler "+args[1])
			h.man.RemoveId("js-" + args[1])

		case args[0] == "lua":
			h.man.conn.Privmsgf(replyTarget(e), "Unregistered Lua handler "+args[1])
			h.man.RemoveId("lua-" + args[1])

		case args[0] == "lisp":
			h.man.conn.Privmsgf(replyTarget(e), "Unregistered Lisp handler "+args[1])
			h.man.RemoveId("lisp-" + args[1])
		}

	case command == "repl":
		if len(args) != 1 && args[0] != "js" && args[0] != "lua" && args[0] != "lisp" {
			h.man.conn.Privmsgf(replyTarget(e), "Invalid syntax. Usage: !repl <js|lua|lisp>")
			return
		}

		h.repl = true
		h.replType = args[0]
		h.man.conn.Privmsgf(replyTarget(e), "%s REPL session started.", replTypePretty(h.replType))
	}
}

func newJavascriptScript(man *Manager, vm *otto.Otto, fn string) *JavascriptScript {
	return &JavascriptScript{man, vm, fn}
}

type JavascriptScript struct {
	man *Manager
	vm  *otto.Otto
	fn  string
}

func (h *JavascriptScript) Id() string {
	return "js-" + h.fn
}

func (h *JavascriptScript) Matches(e *irc.Event) bool {
	return true
}

func runUnsafeJavascript(vm *otto.Otto, unsafe string) (otto.Value, error) {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		if err := recover(); err != nil {
			if err == halt {
				fmt.Println("Some code took too long! Stopping after: ", duration)
			}
			panic(err)
		}
	}()

	vm.Interrupt = make(chan func(), 1)

	go func() {
		time.Sleep(maxExecutionTime * time.Second)
		vm.Interrupt <- func() {
			panic(halt)
		}
	}()

	return vm.Run(unsafe)
}

func (h *JavascriptScript) Handle(e *irc.Event) {
	defer scriptRecoveryHandler(h.man, e)

	h.vm.Set("print", func(call otto.FunctionCall) otto.Value {
		message, _ := call.Argument(0).ToString()
		h.man.conn.Privmsgf(replyTarget(e), message)
		return otto.Value{}
	})
	_, err := runUnsafeJavascript(h.vm, fmt.Sprintf("%s(\"%s\", \"%s\", \"%s\")", h.fn, e.Arguments[0], e.Nick, e.Message()))
	if err != nil {
		h.man.conn.Privmsgf(replyTarget(e), err.Error())

		return
	}
}

func newLuaScript(man *Manager, vm *lua.State, fn string) *LuaScript {
	return &LuaScript{man, vm, fn}
}

type LuaScript struct {
	man *Manager
	vm  *lua.State
	fn  string
}

func (h *LuaScript) Id() string {
	return "lua-" + h.fn
}

func (h *LuaScript) Matches(e *irc.Event) bool {
	return true
}

func runUnsafeLua(vm *lua.State, unsafe string) error {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		if err := recover(); err != nil {
			if err == halt {
				fmt.Println("Some code took too long! Stopping after: ", duration)
			}
			panic(err)
		}
	}()

	vm.SetExecutionLimit(maxExecutionTime * (1 << 26))
	err := vm.DoString(unsafe)

	if err.Error() == "Lua execution quantum exceeded" {
		panic(halt)
	}

	return err
}

func (h *LuaScript) Handle(e *irc.Event) {
	defer scriptRecoveryHandler(h.man, e)

	h.vm.Register("print", func(vm *lua.State) int {
		o := vm.ToString(1)
		h.man.conn.Privmsgf(replyTarget(e), o)
		return 0
	})
	err := runUnsafeLua(h.vm, fmt.Sprintf("%s(\"%s\", \"%s\", \"%s\")", h.fn, e.Arguments[0], e.Nick, e.Message()))
	if err != nil {
		h.man.conn.Privmsgf(replyTarget(e), err.Error())
	}
}

func newLispScript(man *Manager, fn string) *LispScript {
	return &LispScript{man, fn}
}

type LispScript struct {
	man *Manager
	fn  string
}

func (h *LispScript) Id() string {
	return "lisp-" + h.fn
}

func (h *LispScript) Matches(e *irc.Event) bool {
	return true
}

func runUnsafeLisp(unsafe string) (lisp.Value, error) {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		if err := recover(); err != nil {
			if err.(error).Error() == "Execution limit exceeded" {
				fmt.Println("Some code took too long! Stopping after: ", duration)
				panic(halt)
			}
			panic(err)
		}
	}()

	lisp.SetExecutionLimit(maxExecutionTime * (1 << 15))
	return lisp.EvalString(unsafe)
}

func (h *LispScript) Handle(e *irc.Event) {
	defer scriptRecoveryHandler(h.man, e)

	lisp.SetHandler("print", func(vars ...lisp.Value) (lisp.Value, error) {
		if len(vars) == 1 {
			h.man.conn.Privmsgf(replyTarget(e), vars[0].String())
		}
		return lisp.Nil, nil
	})
	_, err := runUnsafeLisp(fmt.Sprintf("(%s \"%s\" \"%s\" \"%s\")", h.fn, e.Arguments[0], e.Nick, e.Message()))

	if err == halt {
		panic(err)

	} else if err != nil {
		h.man.conn.Privmsgf(replyTarget(e), err.Error())

		return
	}
}
