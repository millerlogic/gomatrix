package gomatrix

import (
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/gdamore/tcell"
)

// Stream updates a StreamDisplay with new data updates
type Stream struct {
	display  *StreamDisplay
	speed    int
	length   int
	headPos  int
	tailPos  int
	stopCh   chan struct{}
	headDone bool
}

func (s *Stream) run() {
	blackStyle := tcell.StyleDefault.
		Foreground(tcell.ColorBlack).
		Background(tcell.ColorBlack)

	midStyleA := blackStyle.Foreground(tcell.ColorGreen)
	midStyleB := blackStyle.Foreground(tcell.ColorLime)
	headStyleA := blackStyle.Foreground(tcell.ColorSilver)
	headStyleB := blackStyle.Foreground(tcell.ColorWhite)

	var lastRune rune
STREAM:
	for {
		gLock.RLock()
		curSizesHeight := curSizes.height
		gLock.RUnlock()

		select {
		case <-s.stopCh:
			log.Printf("Stream on SD %d was stopped.\n", s.display.column)
			break STREAM
		case <-time.After(time.Duration(s.speed) * time.Millisecond):
			// add a new rune if there is space in the stream
			if !s.headDone && s.headPos <= curSizesHeight {
				newRune := characters[rand.Intn(len(characters))]

				// Making most of the green characters bright/bold...
				if rand.Intn(100) < 66 {
					screen.SetCell(s.display.column, s.headPos-1, midStyleA, lastRune)
				} else {
					screen.SetCell(s.display.column, s.headPos-1, midStyleB, lastRune)
				}

				// ...and turning about a third of the heads from gray to white
				if rand.Intn(100) < 33 {
					screen.SetCell(s.display.column, s.headPos, headStyleA, newRune)
				} else {
					screen.SetCell(s.display.column, s.headPos, headStyleB, newRune)
				}
				lastRune = newRune
				s.headPos++
			} else {
				s.headDone = true
			}

			// clear rune at the tail of the stream
			if s.tailPos > 0 || s.headPos >= s.length || s.headPos > curSizesHeight {
				if s.tailPos == 0 {
					// tail is being incremented for the first time. there is space for a new stream
					s.display.streamsLock.Lock()
					newStream := s.display.newStream
					if newStream == nil {
						s.display.streamsLock.Unlock()
						break STREAM
					}
					s.display.streamsLock.Unlock()
					newStream <- true
				}
				if s.tailPos < curSizesHeight {
					screen.SetCell(s.display.column, s.tailPos, blackStyle, ' ') //'\uFF60'
					s.tailPos++
				} else {
					break STREAM
				}
			}
		}
	}

	s.display.streamsLock.Lock()
	delete(s.display.streams, s)
	s.display.streamsLock.Unlock()
}

// StreamDisplay represents a vertical line in the terminal on which `Stream`s are displayed.
// StreamDisplay also creates the Streams themselves
type StreamDisplay struct {
	column      int
	stopCh      chan struct{}
	streams     map[*Stream]bool
	streamsLock sync.Mutex
	newStream   chan bool
}

func (sd *StreamDisplay) run() {
	for {
		select {
		case <-sd.stopCh:
			sd.streamsLock.Lock()

			for {
				select {
				case <-sd.newStream: // Drop any pending new streams.
					continue
				default:
				}
				break
			}
			sd.newStream = nil // Done.

			// stop streams for this SD
			for s := range sd.streams {
				close(s.stopCh)
			}

			// log that SD has closed
			log.Printf("StreamDisplay on column %d stopped.\n", sd.column)

			sd.streamsLock.Unlock()

			// Wait for all the streams to be done.
			for {
				time.Sleep(10 * time.Millisecond)
				sd.streamsLock.Lock()
				done := len(sd.streams) == 0
				sd.streamsLock.Unlock()
				if done {
					break
				}
			}

			// close this goroutine
			return

		case <-sd.newStream:
			// have some wait before the first stream starts..
			select {
			case <-sd.stopCh:
				continue
			case <-time.After(time.Duration(rand.Intn(9000)) * time.Millisecond):
			}

			// lock map
			sd.streamsLock.Lock()

			// create new stream instance
			s := &Stream{
				display: sd,
				stopCh:  make(chan struct{}),
				speed:   30 + rand.Intn(110),
				length:  10 + rand.Intn(8), // length of a stream is between 10 and 18 runes
			}

			// store in streams map
			sd.streams[s] = true

			// run the stream in a goroutine
			go s.run()

			// unlock map
			sd.streamsLock.Unlock()
		}
	}
}
