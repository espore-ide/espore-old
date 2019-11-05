package session

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sync"
	"time"
)

const throttle = 100 * time.Millisecond
const chunkSize = 128

type Config struct {
	Socket io.ReadWriteCloser
}

type Session struct {
	Config
}

func New(config *Config) (*Session, error) {
	s := &Session{
		Config: *config,
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
	scanner := bufio.NewScanner(s.Socket)
	for i := 0; i < 10; i++ {
		for scanner.Scan() {
			st := scanner.Text()
			if st == search {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("string not found")
}

func (s *Session) AwaitRegex(regexSt string) ([]string, error) {
	scanner := bufio.NewScanner(s.Socket)
	r := regexp.MustCompile(regexSt)

	for i := 0; i < 10; i++ {
		for scanner.Scan() {
			st := scanner.Text()
			match := r.FindStringSubmatch(st)
			if len(match) > 0 {
				return match, nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil, errors.New("regex not found")
}

func (s *Session) pushloader() error {
	if err := s.SendCommand(upbin); err != nil {
		return err
	}

	if err := s.AwaitString("READY"); err != nil {
		return errors.New("Loader failed")
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
	if err := s.ensureRuntime(); err != nil {
		return err
	}
	sw := NewSlowWriter(s.Socket)

	const tmpfile = "upload.tmp"
	if err := s.startUpload(tmpfile, size); err != nil {
		return err
	}

	if err := s.AwaitString("BEGIN"); err != nil {
		return errors.New("Error waiting for upload BEGIN signal")
	}

	wg := new(sync.WaitGroup)
	wg.Add(1)
	var copyErr error
	var hash string
	go func() {
		defer wg.Done()
		hasher := sha1.New()
		reader := io.TeeReader(reader, hasher)
		_, copyErr = io.Copy(sw, reader)
		if copyErr != nil {
			return
		}
		hash = hex.EncodeToString(hasher.Sum(nil))
	}()

	wg.Wait()
	if copyErr != nil {
		return fmt.Errorf("Error pushing file: %s", copyErr)
	}
	if err := s.AwaitString(hash); err != nil {
		return errors.New("Hash mismatch in uploaded file")
	}
	return s.RenameFile(tmpfile, dstName)
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
	return s.pushloader()
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
