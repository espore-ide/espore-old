package cli

import (
	"errors"
	"espore/cli/history"
	"espore/cli/syncer"
	"espore/config"
	"espore/session"
	"fmt"
	"regexp"
	"sync"

	"github.com/epiclabs-io/winman"
	"github.com/gdamore/tcell"
	"github.com/rivo/tview"
)

type Config struct {
	Session      *session.Session
	OnQuit       func()
	EsporeConfig *config.EsporeConfig
	History      *history.History
}

type UI struct {
	Config
	dumper            *Dumper
	app               *tview.Application
	input             *tview.InputField
	output            *tview.TextView
	fileBrowser       *tview.Table
	fileBrowserHidden bool
	outerFlex         *tview.Flex
	innerFlex         *tview.Flex
	wm                *winman.Manager
	mainWnd           *winman.WindowBase
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
		app:               tview.NewApplication(),
		outerFlex:         tview.NewFlex(),
		innerFlex:         tview.NewFlex(),
		output:            tview.NewTextView(),
		input:             tview.NewInputField(),
		wm:                winman.NewWindowManager(),
		fileBrowser:       tview.NewTable(),
		fileBrowserHidden: false,
	}
	ui.commandHandlers = ui.buildCommandHandlers()
	ui.Session.Log = ui
	ui.dumper = &Dumper{
		R: ui.Session,
		W: ui.output,
	}
	ui.mainWnd = ui.wm.NewWindow().
		Show().
		Maximize().
		SetBorder(false)

	return ui
}

func (ui *UI) Printf(format string, a ...interface{}) {
	fmt.Fprintf(ui.output, "[yellow]"+format+"[-]", a...)
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

	if err := ui.app.SetRoot(ui.wm, true).EnableMouse(true).Run(); err != nil {
		panic(err)
	}
	close(ui.commands)

	return appError
}
