package cli

import (
	"espore/initializer"
	"fmt"
)

func (c *CLI) runCode(luaCode string) error {
	return c.Session.SendCommand(fmt.Sprintf(`
(function ()
%s
end)()
`, luaCode))

}

func (c *CLI) ls() error {
	return c.runCode(`
	print("")	
	for name,size in pairs(file.list()) do
		print(name .. "\t" .. size)
	end
`)
}

func (c *CLI) unload(packageName string) error {
	if packageName == "*" {
		return c.runCode(`
		__loader.unloadAll()
		print("\nAll packages unloaded")
		`)
	}
	return c.runCode(fmt.Sprintf(`
		__loader.unload("%s")
		print("\nUnloaded %s")
		`, packageName, packageName))
}

func (c *CLI) push(srcPath, dstPath string) error {
	return c.Session.PushFile(srcPath, dstPath)
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
				c.dumper.Stop()
				defer c.dumper.Dump()
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
	}
}
