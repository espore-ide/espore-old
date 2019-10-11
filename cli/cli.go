package cli

import (
	"errors"
	"espore/session"
	"fmt"
	"regexp"
	"strings"

	"github.com/gdamore/tcell"
	"github.com/rivo/tview"
)

type Config struct {
	Session *session.Session
	OnQuit  func()
}

type commandHandler struct {
	handler       func(parameters []string) error
	minParameters int
}

type CLI struct {
	Config
	dumper          *Dumper
	app             *tview.Application
	input           *tview.InputField
	textView        *tview.TextView
	commandHandlers map[string]*commandHandler
}

var commandRegex = regexp.MustCompile(`(?m)^\/([^ ]*) *(.*)$`)
var errQuit = errors.New("User quit")

func New(config *Config) *CLI {

	cli := &CLI{
		Config: *config,
	}
	cli.commandHandlers = cli.buildCommandHandlers()

	return cli
}

func (c *CLI) parseCommandLine(cmdline string) error {
	match := commandRegex.FindStringSubmatch(cmdline)
	if len(match) > 0 {
		command := match[1]
		parameters := strings.Split(match[2], " ")
		handler := c.commandHandlers[command]
		if handler == nil {
			fmt.Fprintf(c.textView, "Unknown command %q\n", command)
			return nil
		}
		if len(parameters) < handler.minParameters {
			fmt.Fprintf(c.textView, "Expected at least %d parameters. Got %d", handler.minParameters, len(parameters))
			return nil
		}
		return handler.handler(parameters)
	}
	return c.Session.SendCommand(cmdline)
}

func (c *CLI) Run() error {
	var history []string
	var historyPos int

	var appError error
	app := tview.NewApplication()
	flexbox := tview.NewFlex()
	input := tview.NewInputField()

	textView := tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true).
		SetWordWrap(true).
		SetChangedFunc(func() {
			app.Draw()
		}).
		SetScrollable(true).
		ScrollToEnd()

	textView.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyTAB {
			app.SetFocus(input)
		}

	})
	textView.SetBorder(true)

	flexbox.SetDirection(tview.FlexRow)
	flexbox.AddItem(textView, 0, 1, false)
	flexbox.AddItem(input, 1, 0, true)

	input.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyTAB:
			app.SetFocus(textView)
		case tcell.KeyEnter:
			cmd := strings.TrimSpace(input.GetText())
			if len(cmd) == 0 {
				return
			}
			input.SetText("")
			err := c.parseCommandLine(cmd)
			if err != nil {
				appError = err
				app.Stop()
			}
			lh := len(history)
			if lh == 0 || (lh > 0 && history[lh-1] != cmd) {
				history = append(history, cmd)
				historyPos = lh + 1
			}
		}
	})

	input.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyUp:
			if historyPos > 0 {
				historyPos--
				input.SetText(history[historyPos])
			}
			return nil
		case tcell.KeyDown:
			if historyPos < len(history)-1 {
				historyPos++
				input.SetText(history[historyPos])
			} else {
				input.SetText("")
			}
			return nil

		}
		return event
	})

	c.dumper = &Dumper{
		R: c.Session,
		W: textView,
	}
	c.dumper.Dump()
	defer c.dumper.Stop()

	c.app = app
	c.input = input
	c.textView = textView

	if err := app.SetRoot(flexbox, true).Run(); err != nil {
		panic(err)
	}

	return appError
}
