package main

import (
	"bytes"
	"fmt"
	"hash"
	"hash/crc32"
	"sync"
)

func NewTrace() *Trace {
	return &Trace{
		checksum: crc32.NewIEEE(),
	}
}

type Trace struct {
	b        bytes.Buffer
	checksum hash.Hash32
	offset   int
	m        sync.Mutex
	readMu   sync.Mutex
	lastRead int
}

func (t *Trace) Write(p []byte) (n int, err error) {
	t.m.Lock()
	defer t.m.Unlock()
	t.checksum.Write(p)
	return t.b.Write(p)
}

func (t *Trace) NextChunk() ([]byte, int) {
	t.readMu.Lock()
	t.m.Lock()
	defer t.m.Unlock()
	outAlias := t.b.Bytes()[t.offset:]
	t.lastRead = len(outAlias)
	out := make([]byte, len(outAlias))
	copy(out, outAlias)
	return out, t.offset
}

func (t *Trace) CommitChunk() {
	t.m.Lock()
	defer t.m.Unlock()
	t.offset += t.lastRead
	t.readMu.Unlock()
}

func (t *Trace) AbortChunk() {
	t.m.Lock()
	defer t.m.Unlock()
	t.lastRead = 0
	t.readMu.Unlock()
}

func (t *Trace) Offset() int {
	t.m.Lock()
	defer t.m.Unlock()
	return t.offset
}

func (t *Trace) Checksum() string {
	t.m.Lock()
	defer t.m.Unlock()
	return fmt.Sprintf("crc32:%08x", t.checksum.Sum32())
}
