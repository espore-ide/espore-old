package cli

import (
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

func (c *CLI) ls() error {
	return c.Session.RunCode(`__espore.ls()`)
}

func (c *CLI) unload(packageName string) error {
	if packageName == "*" {
		return c.Session.RunCode(`
		__espore.unloadAll()
		print("\nAll packages unloaded")
		`)
	}
	return c.Session.RunCode(fmt.Sprintf(`
		__espore.unload("%s")
		print("\nUnloaded %s")
		`, packageName, packageName))
}

func (c *CLI) push(srcPath, dstPath string) error {
	c.Printf("Uploading %s to %s ... ", srcPath, dstPath)
	err := c.Session.PushFile(srcPath, dstPath)
	if err != nil {
		c.Printf("Error uploading file: %s\n", err)
	} else {
		c.Printf("OK\n")
	}
	return nil
}

func (c *CLI) watch(srcPath, dstPath string) error {
	currentDir, err := os.Getwd()
	if err != nil {
		return err
	}
	srcPath = filepath.Join(currentDir, srcPath)
	sync := c.syncers[srcPath]
	if sync != nil {
		sync.Close()
		delete(c.syncers, srcPath)
	}

	sync, err = syncer.New(&syncer.Config{
		SrcPath: srcPath,
		OnSync: func(path string) {
			c.app.QueueUpdate(func() {
				relFile, err := filepath.Rel(srcPath, path)
				if err != nil {
					c.Printf("[red]Error pushing file: %s\n", err)
				} else {
					dstName := filepath.Join(dstPath, relFile)
					c.dumper.Stop()
					defer c.dumper.Dump()
					err = c.Session.PushFile(path, dstName)
					if err != nil {
						c.Printf("[red]Error pushing %s: %s[-:-:-]\n", dstName, err)
					} else {
						c.Printf("Pushed %s\n", dstName)
					}
				}
			})
		},
	})
	if err != nil {
		c.Printf("Error setting up sync for %s->%s: %s\n", srcPath, dstPath, err)
	} else {
		c.Printf("Watching %s for changes\n", srcPath)
	}

	return nil
}

func (c *CLI) cat(path string) error {
	//TODO: encode somehow so as to avoid the newlines in print()
	return c.Session.RunCode(fmt.Sprintf(`
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

func (c *CLI) buildCommandHandlers() map[string]*commandHandler {
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
				return c.ls()
			},
		},
		"init": &commandHandler{
			minParameters: 0,
			handler: func(p []string) error {
				return initializer.Initialize(c.Session)
			},
		},
		"unload": &commandHandler{
			minParameters: 1,
			handler: func(p []string) error {
				return c.unload(p[0])
			},
		},
		"push": &commandHandler{
			minParameters: 2,
			handler: func(p []string) error {
				return c.push(p[0], p[1])
			},
		},
		"clear": &commandHandler{
			handler: func(p []string) error {
				c.textView.SetText("")
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
				return c.watch(p[0], dstPath)
			},
		},
		"cat": &commandHandler{
			minParameters: 1,
			handler: func(p []string) error {
				return c.cat(p[0])
			},
		},
		"restart": &commandHandler{
			handler: func(p []string) error {
				return c.Session.NodeRestart()
			},
		},
		"build": &commandHandler{
			handler: func(p []string) error {
				err := builder.Build()
				if err == nil {
					c.Printf("Firmware images built.\n")
				}
				return err
			},
		},
	}
}
