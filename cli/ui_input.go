package cli

import (
	"sort"
	"strings"

	"github.com/gdamore/tcell/v2"
)

func (ui *UI) initInput() {
	input := ui.input

	input.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyTAB:
			ui.app.SetFocus(ui.output)
		case tcell.KeyEnter:
			cmd := strings.TrimSpace(input.GetText())
			if len(cmd) == 0 {
				return
			}
			ui.input.SetText("")
			ui.commands <- func() {
				err := ui.parseCommandLine(cmd)
				if err != nil {
					ui.Printf("Error executing command: %s", err)
				}
			}
			ui.History.Append(cmd)
		}
	})

	input.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyUp:
			input.SetText(ui.History.Up())
			return nil
		case tcell.KeyDown:
			input.SetText(ui.History.Down())
			return nil
		}
		return event
	})

	var commands []string
	for c := range ui.commandHandlers {
		commands = append(commands, c)
	}
	sort.Slice(commands, func(i, j int) bool {
		return strings.Compare(commands[i], commands[j]) < 0
	})

	input.SetAutocompleteFunc(func(currentText string) []string {
		if len(currentText) == 0 {
			return nil
		}
		var entries []string
		for _, c := range commands {
			cmd := "/" + c
			if strings.HasPrefix(cmd, currentText) {
				entries = append(entries, cmd)
			}
		}
		return entries
	})
}

func (ui *UI) parseCommandLine(cmdline string) error {
	match := commandRegex.FindStringSubmatch(cmdline)
	if len(match) > 0 {
		command := match[1]
		parameters := strings.Split(match[2], " ")
		handler := ui.commandHandlers[command]
		if handler == nil {
			ui.Printf("Unknown command %q\n", command)
			return nil
		}
		if len(parameters) < handler.minParameters {
			ui.Printf("Expected at least %d parameters. Got %d\n", handler.minParameters, len(parameters))
			return nil
		}
		return handler.handler(parameters)
	}
	return ui.Session.SendCommand(cmdline)
}
