package logbus

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// LogBus is a process-wide ring buffer of log lines, mirroring Android's LogBus.
// UI, Go core, and tun2socks logs all funnel here. Capped at MaxLines entries.
// Subscribers receive new lines in real time via Subscribe.
type LogBus struct {
	mu       sync.Mutex
	lines    []string
	maxLines int
	subs     []chan string
}

const defaultMaxLines = 200

var Default = &LogBus{maxLines: defaultMaxLines}

// Post appends a timestamped line to the bus and notifies subscribers.
func (b *LogBus) Post(line string) {
	ts := time.Now().Format("15:04:05.000")
	entry := fmt.Sprintf("[%s] %s", ts, line)
	b.mu.Lock()
	b.lines = append(b.lines, entry)
	if len(b.lines) > b.maxLines {
		b.lines = b.lines[len(b.lines)-b.maxLines:]
	}
	subs := make([]chan string, len(b.subs))
	copy(subs, b.subs)
	b.mu.Unlock()

	// Non-blocking send to subscribers
	for _, ch := range subs {
		select {
		case ch <- entry:
		default:
		}
	}
}

// Lines returns a copy of the current log lines.
func (b *LogBus) Lines() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]string, len(b.lines))
	copy(out, b.lines)
	return out
}

// Clear empties the bus.
func (b *LogBus) Clear() {
	b.mu.Lock()
	b.lines = b.lines[:0]
	b.mu.Unlock()
}

// Subscribe returns a channel that receives new log entries. The channel is
// buffered (64). Call Unsubscribe to stop and release the channel.
func (b *LogBus) Subscribe() chan string {
	ch := make(chan string, 64)
	b.mu.Lock()
	b.subs = append(b.subs, ch)
	b.mu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber channel.
func (b *LogBus) Unsubscribe(ch chan string) {
	b.mu.Lock()
	for i, s := range b.subs {
		if s == ch {
			b.subs = append(b.subs[:i], b.subs[i+1:]...)
			break
		}
	}
	b.mu.Unlock()
	close(ch)
}

// Postf is a printf-style Post.
func (b *LogBus) Postf(format string, args ...interface{}) {
	b.Post(fmt.Sprintf(format, args...))
}

// PostGlobal posts to the default bus.
func PostGlobal(line string) { Default.Post(line) }
func PostGlobalf(format string, a ...interface{}) { Default.Postf(format, a...) }

// LinesGlobal returns lines from the default bus.
func LinesGlobal() []string { return Default.Lines() }

// ClearGlobal clears the default bus.
func ClearGlobal() { Default.Clear() }

// JoinLines returns all lines joined with newlines (for clipboard copy).
func JoinLines() string { return strings.Join(LinesGlobal(), "\n") }