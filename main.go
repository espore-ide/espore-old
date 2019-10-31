package main

import (
	"espore/builder"
	"espore/cli"
	"espore/fwserver"
	"espore/initializer"
	"espore/session"
	"flag"
	"log"
	"time"

	"github.com/tarm/serial"
)

func getSerialSession() (s *session.Session, close func(), err error) {
	socket, err := serial.OpenPort(&serial.Config{Name: "/dev/ttyUSB0", Baud: 115200, ReadTimeout: time.Second * 1})
	if err != nil {
		return nil, nil, err
	}

	s, err = session.New(&session.Config{
		Socket: socket,
	})
	if err != nil {
		socket.Close()
		return nil, nil, err
	}

	return s, func() {
		s.Close()
		socket.Close()
	}, nil

}

func initFirmware() error {
	s, close, err := getSerialSession()
	if err != nil {
		return err
	}

	defer close()
	return initializer.Initialize(s)
}

func main() {
	watchFlag := flag.Bool("watch", false, "Watch for changes")
	initFlag := flag.Bool("initialize", false, "Initialize device")
	cliFlag := flag.Bool("cli", false, "Run the CLI")
	serverFlag := flag.Bool("server", false, "Run the firmware server")

	flag.Parse()
	err := builder.Build()
	if err != nil {
		log.Fatal(err)
	}

	if *initFlag {
		if err := initFirmware(); err != nil {
			log.Fatal(err)
		}
	}

	if *watchFlag {
		watch()
		return
	}

	if *serverFlag {
		fwserver.New(&fwserver.Config{
			Port: 8080,
			Base: "dist",
		})
	}

	if *cliFlag {
		session, close, err := getSerialSession()
		if err != nil {
			log.Fatalf("Error opening session over serial: %s", err)
		}
		defer close()
		c := cli.New(&cli.Config{
			Session: session,
		})

		err = c.Run()
		if err != nil {
			log.Fatalf("CLI:%s", err)
		}
	}
}
