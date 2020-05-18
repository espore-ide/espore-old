package cli

import (
	"errors"
	"espore/builder"
	"espore/cli/syncer"
	"espore/session"
	"fmt"
	"regexp"
	"sync"

	"github.com/gdamore/tcell"
	"gitlab.com/tslocum/cview"
)

type Config struct {
	Session     *session.Session
	OnQuit      func()
	BuildConfig *builder.BuildConfig
}

type UI struct {
	Config
	dumper            *Dumper
	app               *cview.Application
	input             *cview.InputField
	output            *cview.TextView
	fileBrowser       *cview.Table
	fileBrowserHidden bool
	outerFlex         *cview.Flex
	innerFlex         *cview.Flex
	commandHandlers   map[string]*commandHandler
	syncers           map[string]*syncer.Syncer
	commands          chan func()
}

var commandRegex = regexp.MustCompile(`(?m)^\/([^ ]*) *(.*)$`)
var errQuit = errors.New("User quit")

const MAX_TEXT_BUFFER = 10000

func New(config *Config) *UI {

	ui := &UI{
		Config:            *config,
		syncers:           make(map[string]*syncer.Syncer),
		commands:          make(chan func(), 10),
		app:               cview.NewApplication(),
		outerFlex:         cview.NewFlex(),
		innerFlex:         cview.NewFlex(),
		output:            cview.NewTextView(),
		input:             cview.NewInputField(),
		fileBrowser:       cview.NewTable(),
		fileBrowserHidden: false,
	}
	ui.commandHandlers = ui.buildCommandHandlers()
	ui.Session.Log = ui
	ui.dumper = &Dumper{
		R: ui.Session,
		W: ui.output,
	}

	return ui
}

func (ui *UI) Printf(format string, a ...interface{}) {
	fmt.Fprintf(ui.output, format, a...)
	/*	c.app.QueueUpdateDraw(func() {
		fmt.Printf("Q: %s", format)
	})*/
}

func (ui *UI) Run() error {

	var appError error

	ui.initInput()
	ui.initOutput()
	ui.initFileBrowser()
	ui.initLayout()

	go func() {
		wg := sync.WaitGroup{}
		for cmdFunc := range ui.commands {
			wg.Add(1)
			ui.app.QueueUpdate(func() {
				go func() {
					defer wg.Done()
					cmdFunc()
				}()
			})
			wg.Wait()
		}

	}()

	ui.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyCtrlB:
			if ui.fileBrowserHidden {
				ui.fileBrowserHidden = false
				ui.innerFlex.ResizeItem(ui.fileBrowser, 20, 0)
			} else {
				ui.fileBrowserHidden = true
				ui.innerFlex.ResizeItem(ui.fileBrowser, 0, 0)
			}
		}
		return event
	})

	ui.dumper.Dump()
	defer ui.dumper.Stop()

	if err := ui.app.SetRoot(ui.outerFlex, true).Run(); err != nil {
		panic(err)
	}
	close(ui.commands)

	return appError
}
