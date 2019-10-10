package cli

import (
	"errors"
	"espore/initializer"
	"espore/session"
	"regexp"
	"strings"

	"github.com/gdamore/tcell"
	"github.com/rivo/tview"
)

type Config struct {
	Session *session.Session
	OnQuit  func()
}

type CLI struct {
	Config
	dumper *Dumper
}

var commandRegex = regexp.MustCompile(`(?m)^\/([^ ]*) *(.*)$`)
var errQuit = errors.New("User quit")

func New(config *Config) *CLI {

	cli := &CLI{
		Config: *config,
	}
	return cli
}

func (c *CLI) parseCommandLine(cmdline string) error {
	match := commandRegex.FindStringSubmatch(cmdline)
	if len(match) > 0 {
		switch match[1] {
		case "quit":
			return errQuit
		case "ls":
			return c.ls()
		case "init":
			c.dumper.Stop()
			defer c.dumper.Dump()
			return initializer.Initialize(c.Session)
		case "unload":
			return c.unload(match[2])
		}
	} else {
		return c.Session.SendCommand(cmdline)
	}
	return nil
}

func (c *CLI) Run() error {
	/*
		esc := escape.New(&escape.Config{
			Reader:   os.Stdin,
			Sequence: []byte{27, 91, 65},
			Callback: func() {
				fmt.Println("UP!!")
			},
		})
	*/
	/*
		console := bufio.NewReader(os.Stdin)
		fmt.Println("CLI ready")

		for {
			cmdline, err := console.ReadString('\n')
			if err != nil {
				return fmt.Errorf("Error reading from console: %s", err)
			}
			fmt.Println([]byte(cmdline))
			err = c.parseCommandLine(cmdline)
			if err != nil {
				return fmt.Errorf("Error parsing command line: %s", err)
			}
		}
	*/
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

	if err := app.SetRoot(flexbox, true).Run(); err != nil {
		panic(err)
	}

	return appError
}
