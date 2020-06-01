package main

import (
	"bytes"
	"espore/builder"
	"espore/cli"
	"espore/cli/history"
	"espore/config"
	"espore/fwserver"
	"espore/initializer"
	"espore/session"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
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

func buildHistory(fileName string) (*history.History, error) {
	var r io.Reader
	f, err := os.Open(fileName)
	if err != nil {
		r = bytes.NewBufferString("")
	} else {
		r = f
	}

	return history.New(r, &history.Config{
		Limit: 100,
		OnAppend: func(line string) {
			f, err := os.OpenFile(fileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				return
			}
			defer f.Close()
			_, err = fmt.Fprintln(f, line)
			if err != nil {
				fmt.Println(err)
			}
		},
	})
}

func main() {
	watchFlag := flag.Bool("watch", false, "Watch for changes")
	initFlag := flag.Bool("initialize", false, "Initialize device")
	cliFlag := flag.Bool("cli", false, "Run the interactive UI")
	serverFlag := flag.Bool("server", false, "Run the firmware server")

	flag.Parse()

	config, err := config.Read()
	if err != nil {
		log.Printf("Error: %s", err)
	}

	dataDir := config.GetDataDir()
	os.MkdirAll(dataDir, 0755)

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

		historyFileName := filepath.Join(dataDir, "history.txt")
		history, err := buildHistory(historyFileName)
		if err != nil {
			log.Fatalf("Error reading history: %s", err)
		}

		c := cli.New(&cli.Config{
			Session:      session,
			EsporeConfig: config,
			History:      history,
		})

		err = c.Run()
		if err != nil {
			log.Fatalf("CLI:%s", err)
		}
	}
	err = builder.Build(&config.Build)
	if err != nil {
		log.Fatal(err)
	}

	if *initFlag {
		if err := initFirmware(); err != nil {
			log.Fatal(err)
		}
	}

	if *watchFlag {
		watch(config)
		return
	}
}
