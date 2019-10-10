package cli

import (
	"bufio"
	"errors"
	"espore/initializer"
	"espore/session"
	"fmt"
	"os"
	"regexp"
)

type Config struct {
	Session *session.Session
	OnQuit  func()
}

type CLI struct {
	Config
	dumper *Dumper
}

var commandRegex = regexp.MustCompile(`(?m)^\/([^ ]*) *(.*)\n$`)
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
	console := bufio.NewReader(os.Stdin)
	c.dumper = &Dumper{
		R: c.Session,
	}
	fmt.Println("CLI ready\n")
	c.dumper.Dump()
	defer c.dumper.Stop()
	for {
		cmdline, err := console.ReadString('\n')
		if err != nil {
			return fmt.Errorf("Error reading from console: %s", err)
		}
		err = c.parseCommandLine(cmdline)
		if err != nil {
			return fmt.Errorf("Error parsing command line: %s", err)
		}
	}
}
