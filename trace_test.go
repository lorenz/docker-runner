package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTrace(t *testing.T) {
	tr := NewTrace()
	chunk, off := tr.NextChunk()
	assert.Len(t, chunk, 0, "Without writes NextChunk should return a zero-length object")
	assert.Equal(t, 0, off, "The first offset should be zero")
	tr.CommitChunk()
	testVal1 := []byte("Test1")
	testVal2 := []byte("Test2")
	tr.Write(testVal1)
	chunk, off = tr.NextChunk()
	tr.Write(testVal2)
	assert.Equal(t, testVal1, chunk, "NextChunk should return write call value")
	assert.Equal(t, off, 0, "Offset should still be zero")
	tr.CommitChunk()
	chunk, off = tr.NextChunk()
	assert.Equal(t, testVal2, chunk, "NextChunk should return write call value")
	assert.Equal(t, off, len(testVal1), "Offset should be the first chunk")
	tr.AbortChunk()
	chunk, off = tr.NextChunk()
	assert.Equal(t, testVal2, chunk, "NextChunk should return write call value")
	assert.Equal(t, off, len(testVal1), "Offset should be the first chunk")
	tr.CommitChunk()
}
