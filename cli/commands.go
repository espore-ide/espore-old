package cli

import "fmt"

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

	} else {
		return c.runCode(fmt.Sprintf(`
		__loader.unload("%s")
		print("\nUnloaded %s")
		`, packageName, packageName))
	}
}
