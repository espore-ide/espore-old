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

const throttle = 20 * time.Millisecond
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

	if err := s.pushloader(); err != nil {
		return nil, err
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
	for scanner.Scan() {
		st := scanner.Text()
		fmt.Println(st)
		if st == search {
			return nil
		}
	}
	return errors.New("not found")
}

func (s *Session) AwaitRegex(regexSt string) ([]string, error) {
	scanner := bufio.NewScanner(s.Socket)
	r := regexp.MustCompile(regexSt)
	for scanner.Scan() {
		st := scanner.Text()
		fmt.Println(st)
		match := r.FindStringSubmatch(st)
		if len(match) > 0 {
			return match, nil
		}
	}
	return nil, errors.New("not found")
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
	if err := s.SendCommand(fmt.Sprintf("__loader.upload(\"%s\", %d)\n", fname, size)); err != nil {
		return err
	}
	return nil
}

func (s *Session) PushStream(reader io.Reader, size int64, dstName string) error {
	sw := NewSlowWriter(s.Socket)

	if err := s.startUpload(dstName, size); err != nil {
		return err
	}

	s.AwaitString("BEGIN")

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

	s.AwaitString("0")
	wg.Wait()
	s.AwaitString(hash)
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
	return s.SendCommand("\n__loader.finish()\n")
}

func (s *Session) GetChipID() (string, error) {
	if err := s.SendCommand("\nprint('id=' .. node.chipid())\n"); err != nil {
		return "", err
	}

	match, err := s.AwaitRegex("id=(.*)")
	if err != nil {
		return "", err
	}
	return match[1], nil
}

func (s *Session) Read(p []byte) (int, error) {
	return s.Socket.Read(p)
}
