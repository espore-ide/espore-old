package cli

import (
	"encoding/json"
	"errors"
	"espore/builder"
	"espore/cli/syncer"
	"espore/initializer"
	"fmt"
	"os"
	"path/filepath"
)

type commandHandler struct {
	handler       func(parameters []string) error
	minParameters int
}

func (ui *UI) ls() error {
	r, err := ui.Session.Rpc(`return file.list()`)
	if err != nil {
		return err
	}
	var list map[string]int
	if err := json.Unmarshal(r, &list); err != nil {
		return errors.New("Error decoding file list")
	}
	ui.Printf("Files:\n")
	for name, length := range list {
		ui.Printf("%s\t%d\n", name, length)
	}
	return nil
}

func (ui *UI) unload(packageName string) error {
	if packageName == "*" {
		return ui.Session.RunCode(`
		__espore.unloadAll()
		print("\nAll packages unloaded")
		`)
	}
	return ui.Session.RunCode(fmt.Sprintf(`
		__espore.unload("%s")
		print("\nUnloaded %s")
		`, packageName, packageName))
}

func (ui *UI) push(srcPath, dstPath string) error {
	err := ui.Session.PushFile(srcPath, dstPath)
	if err != nil {
		ui.Printf("Error uploading file: %s\n", err)
	} else {
		ui.Printf("OK\n")
	}
	return nil
}

func (ui *UI) watch(srcPath, dstPath string) error {
	currentDir, err := os.Getwd()
	if err != nil {
		return err
	}
	srcPath = filepath.Join(currentDir, srcPath)
	sync := ui.syncers[srcPath]
	if sync != nil {
		sync.Close()
		delete(ui.syncers, srcPath)
	}

	sync, err = syncer.New(&syncer.Config{
		SrcPath: srcPath,
		OnSync: func(path string) {
			ui.app.QueueUpdate(func() {
				relFile, err := filepath.Rel(srcPath, path)
				if err != nil {
					ui.Printf("[red]Error pushing file: %s\n", err)
				} else {
					dstName := filepath.Join(dstPath, relFile)
					ui.dumper.Stop()
					defer ui.dumper.Dump()
					err = ui.Session.PushFile(path, dstName)
					if err != nil {
						ui.Printf("[red]Error pushing %s: %s[-:-:-]\n", dstName, err)
					} else {
						ui.Printf("Pushed %s\n", dstName)
					}
				}
			})
		},
	})
	if err != nil {
		ui.Printf("Error setting up sync for %s->%s: %s\n", srcPath, dstPath, err)
	} else {
		ui.Printf("Watching %s for changes\n", srcPath)
	}

	return nil
}

func (ui *UI) cat(path string) error {
	//TODO: encode somehow so as to avoid the newlines in print()
	return ui.Session.RunCode(fmt.Sprintf(`
	local f = file.open("%s", "r")
	if f then
		local st = f:readline()
		while st ~= nil do
			print(st:sub(1,#st-1))
			st = f:readline()	
		end
	end
	`, path))
}

func (ui *UI) install_runtime() error {
	return ui.Session.InstallRuntime()
}

func (ui *UI) buildCommandHandlers() map[string]*commandHandler {
	return map[string]*commandHandler{
		"quit": &commandHandler{
			minParameters: 0,
			handler: func(p []string) error {
				return errQuit
			},
		},
		"ls": &commandHandler{
			minParameters: 0,
			handler: func(p []string) error {
				return ui.ls()
			},
		},
		"init": &commandHandler{
			minParameters: 0,
			handler: func(p []string) error {
				return initializer.Initialize(ui.Session)
			},
		},
		"install-runtime": &commandHandler{
			minParameters: 0,
			handler: func(p []string) error {
				return ui.install_runtime()
			},
		},
		"unload": &commandHandler{
			minParameters: 1,
			handler: func(p []string) error {
				return ui.unload(p[0])
			},
		},
		"push": &commandHandler{
			minParameters: 2,
			handler: func(p []string) error {
				return ui.push(p[0], p[1])
			},
		},
		"clear": &commandHandler{
			handler: func(p []string) error {
				ui.output.SetText("")
				return nil
			},
		},
		"watch": &commandHandler{
			minParameters: 1,
			handler: func(p []string) error {
				var dstPath string
				if len(p) > 1 {
					dstPath = p[1]
				}
				return ui.watch(p[0], dstPath)
			},
		},
		"cat": &commandHandler{
			minParameters: 1,
			handler: func(p []string) error {
				return ui.cat(p[0])
			},
		},
		"restart": &commandHandler{
			handler: func(p []string) error {
				return ui.Session.NodeRestart()
			},
		},
		"build": &commandHandler{
			handler: func(p []string) error {
				err := builder.Build(ui.Config.BuildConfig)
				if err == nil {
					ui.Printf("Firmware images built.\n")
				}
				return err
			},
		},
	}
}
