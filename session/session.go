package session

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strconv"
	"sync"
	"time"
)

const throttle = 100 * time.Millisecond
const chunkSize = 128

type Logger interface {
	Printf(fmt string, item ...interface{})
}
type Config struct {
	Socket io.ReadWriteCloser
}

type Session struct {
	Config
	Log     Logger
	scanner *bufio.Scanner
}

type defaultLogger struct{}

func (dl *defaultLogger) Printf(fmt string, item ...interface{}) {
	log.Printf(fmt, item...)
}

func New(config *Config) (*Session, error) {
	s := &Session{
		Config:  *config,
		Log:     &defaultLogger{},
		scanner: bufio.NewScanner(config.Socket),
	}

	return s, nil
}

func (s *Session) SendCommand(cmd string) error {
	sw := NewLineWriter(s.Socket)
	_, err := sw.Write([]byte(cmd))
	if err != nil {
		return err
	}
	return nil
}

func (s *Session) AwaitString(search string) error {
	for i := 0; i < 20; i++ {
		for s.scanner.Scan() {
			st := s.scanner.Text()
			if st == search {
				return nil
			}
			i = 0
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("string not found")
}

func (s *Session) AwaitRegex(regexSt string) ([]string, error) {
	r := regexp.MustCompile(regexSt)

	for i := 0; i < 20; i++ {
		for s.scanner.Scan() {
			st := s.scanner.Text()
			match := r.FindStringSubmatch(st)
			if len(match) > 0 {
				return match, nil
			}
			i = 0
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil, errors.New("regex not found")
}

func (s *Session) pushRuntime() error {
	var err error
	defer func() {
		if err == nil {
			s.Log.Printf("OK\n")
		} else {
			s.Log.Printf("ERROR\n")
		}
	}()

	s.Log.Printf("Pushing espore runtime ...")
	if err = s.SendCommand(upbin); err != nil {
		return err
	}

	if err = s.AwaitString("READY"); err != nil {
		return errors.New("Pushing runtime failed")
	}
	return nil
}

func (s *Session) startUpload(fname string, size int64) error {
	if err := s.SendCommand(fmt.Sprintf("__espore.upload(\"%s\", %d)\n", fname, size)); err != nil {
		return err
	}
	return nil
}

func (s *Session) NodeRestart() error {
	return s.RunCode("node.restart()")
}

func (s *Session) RenameFile(oldName, newName string) error {
	s.RunCode(fmt.Sprintf("__espore.rename(%q, %q)", oldName, newName))
	r, err := s.AwaitRegex("RENAME_(OK|FAIL)")
	if err != nil {
		return errors.New("Error waiting for rename file operation")
	}
	if r[1] != "OK" {
		return errors.New("Rename operation failed")
	}
	return nil
}

func (s *Session) PushStream(reader io.Reader, size int64, dstName string) error {
	const tmpfile = "__upload.tmp"
	if err := s.ensureRuntime(); err != nil {
		return err
	}
	s.Log.Printf("Pushing %s ", dstName)
	sw := NewSlowWriter(s.Socket)

	if err := s.startUpload(tmpfile, size); err != nil {
		return err
	}

	if _, err := s.AwaitRegex("BEGIN"); err != nil {
		return errors.New("Error waiting for upload BEGIN signal")
	}

	wg := new(sync.WaitGroup)
	wg.Add(2)
	var copyErr error
	var recvErr error
	var hash string
	var progressCount int64
	rc := make(chan int64)

	go func() {
		hasher := sha1.New()
		reader := io.TeeReader(reader, hasher)
		sent := int64(0)
		buf := make([]byte, chunkSize)
		defer func() {
			hash = hex.EncodeToString(hasher.Sum(nil))
			wg.Done()
		}()

		for {
			received, ok := <-rc
			if !ok {
				return
			}
			if received > progressCount {
				s.Log.Printf(".")
				progressCount += size / 10
			}
			if sent-received == 0 {
				i, err := reader.Read(buf)
				if i > 0 {
					_, copyErr = sw.Write(buf[:i])
					if copyErr != nil {
						return
					}
					sent += int64(i)
				}
				if err != nil {
					if err != io.EOF {
						copyErr = err
					}
					return
				}
			}
		}
	}()
	go func() {
		defer wg.Done()
		defer close(rc)
		var received = int64(0)
		for received < size {
			rc <- received
			st, err := s.AwaitRegex(`^(\d+)$`)
			if err != nil {
				recvErr = fmt.Errorf("Error waiting for download progress response: %s", err)
				return
			}
			received, err = strconv.ParseInt(st[1], 10, 64)
			if err != nil {
				recvErr = fmt.Errorf("Error parsing remaining size: %s", err)
				return
			}
		}
	}()
	wg.Wait()
	if copyErr != nil {
		return fmt.Errorf("Error pushing file: %s", copyErr)
	}
	if recvErr != nil {
		return fmt.Errorf("Error receiving file: %s", recvErr)
	}
	m, err := s.AwaitRegex("([0-9a-fA-F]{40})")
	if err != nil {
		return errors.New("Error waiting for file checksum hash")
	}
	if m[1] != hash {
		return fmt.Errorf("Checksum hash mismatch. Expected %s, got %s", hash, m[1])
	}
	if err := s.RenameFile(tmpfile, dstName); err != nil {
		s.Log.Printf("ERROR\n")
		return err
	}
	s.Log.Printf("OK\n")
	return nil
}

func (s *Session) PushFile(srcPath, dstName string) error {
	file, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := os.Stat(srcPath)
	if err != nil {
		return err
	}
	return s.PushStream(file, info.Size(), dstName)
}

func (s *Session) Close() error {
	return s.SendCommand("\n__espore.finish()\n")
}

func (s *Session) GetChipID() (string, error) {
	if err := s.SendCommand("\nprint('i' .. 'd=' .. node.chipid())\n"); err != nil {
		return "", err
	}

	match, err := s.AwaitRegex("id=(.*)")
	if err != nil {
		return "", err
	}
	return match[1], nil
}

func (s *Session) ensureRuntime() error {
	err := s.SendCommand("\nprint(\"espore=\" .. tostring(__espore ~= nil))\n")
	if err != nil {
		return err
	}
	installedStr, err := s.AwaitRegex("espore=(true|false)")
	if err != nil {
		return errors.New("Error ensuring __espore is installed")
	}
	if installedStr[1] == "true" {
		return nil
	}
	return s.pushRuntime()
}

func (s *Session) RunCode(luaCode string) error {
	if err := s.ensureRuntime(); err != nil {
		return err
	}

	return s.SendCommand(fmt.Sprintf(`
(function ()
%s
end)()
`, luaCode))

}

func (s *Session) Read(p []byte) (int, error) {
	return s.Socket.Read(p)
}
