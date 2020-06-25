package lockreader

import (
	"io"
	"sync"
)

type LockReader struct {
	r    io.Reader
	lock sync.Mutex
}

func New(reader io.Reader) *LockReader {
	return &LockReader{
		r: reader,
	}
}

func (lr *LockReader) Read(data []byte) (i int, err error) {
	err = lr.Lock(func(reader io.Reader) error {
		i, err = reader.Read(data)
		return err
	})
	return i, err
}

func (lr *LockReader) Lock(f func(reader io.Reader) error) error {
	lr.lock.Lock()
	defer lr.lock.Unlock()
	return f(lr.r)
}
