package gomatrix

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"runtime/pprof"
	"sync"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/gdamore/tcell"
)

var screen tcell.Screen

// Opts are options for Run
type Opts struct {
	// display ascii only instead of ascii+kana's
	Ascii bool `short:"a" long:"ascii" description:"Use ascii/alphanumeric characters only instead of a mixture with japanese kana's."`

	// display kana's instead of ascii+kana's
	Kana bool `short:"k" long:"kana" description:"Use japanese kana's only instead of a mix of ascii/alphanumeric and kana's."`

	// enable logging
	Logging bool `short:"l" long:"log" description:"Enable logging debug messages to ~/.gomatrix-log."`

	// enable profiling
	Profile string `short:"p" long:"profile" description:"Write profile to given file path"`

	// FPS
	FPS int `long:"fps" description:"required FPS, must be somewhere between 1 and 60" default:"25"`
}

// array with half width kanas as Go runes
// source: http://en.wikipedia.org/wiki/Half-width_kana
var halfWidthKana = []rune{
	'｡', '｢', '｣', '､', '･', 'ｦ', 'ｧ', 'ｨ', 'ｩ', 'ｪ', 'ｫ', 'ｬ', 'ｭ', 'ｮ', 'ｯ',
	'ｰ', 'ｱ', 'ｲ', 'ｳ', 'ｴ', 'ｵ', 'ｶ', 'ｷ', 'ｸ', 'ｹ', 'ｺ', 'ｻ', 'ｼ', 'ｽ', 'ｾ', 'ｿ',
	'ﾀ', 'ﾁ', 'ﾂ', 'ﾃ', 'ﾄ', 'ﾅ', 'ﾆ', 'ﾇ', 'ﾈ', 'ﾉ', 'ﾊ', 'ﾋ', 'ﾌ', 'ﾍ', 'ﾎ', 'ﾏ',
	'ﾐ', 'ﾑ', 'ﾒ', 'ﾓ', 'ﾔ', 'ﾕ', 'ﾖ', 'ﾗ', 'ﾘ', 'ﾙ', 'ﾚ', 'ﾛ', 'ﾜ', 'ﾝ', 'ﾞ', 'ﾟ',
}

// just basic alphanumeric characters
var alphaNumerics = []rune{
	'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J', 'K', 'L', 'M', 'N', 'O',
	'P', 'Q', 'R', 'S', 'T', 'U', 'V', 'W', 'X', 'Y', 'Z',
	'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'l', 'm', 'n', 'o',
	'p', 'q', 'r', 's', 't', 'u', 'v', 'w', 'x', 'y', 'z',
	'0', '1', '2', '3', '4', '5', '6', '7', '8', '9',
}

// everything together, for that authentic feel
var allTheCharacters = append(halfWidthKana, alphaNumerics...)

// characters to be used, can be set to alphaNumerics or halfWidthKana depending on flags
// defaults to allTheCharacters
var characters []rune

var gLock sync.RWMutex

// current sizes
var curSizes sizes // locked by gLock

// struct sizes contains terminal sizes (in amount of characters)
type sizes struct {
	width                      int
	height                     int
	curStreamsPerStreamDisplay int // current amount of streams per display allowed
}

// set the sizes and notify StreamDisplayManager
func (s *sizes) setSizes(width int, height int) {
	s.width = width
	s.height = height
	s.curStreamsPerStreamDisplay = 1 + height/10
}

// Run the digital rain!
// The screen is optional, set to nil to use default.
// Note: only one can be run at a time, it uses global state.
func Run(opts Opts, useScreen tcell.Screen, output io.Writer) int {
	if screen != nil { // global screen already set?
		fmt.Fprintln(output, "Already running")
		return 1
	}
	screen = useScreen
	defer func() { screen = nil }()

	// setup logging with logfile /dev/null or ~/.gomatrix-log
	filename := os.DevNull
	if opts.Logging {
		filename = os.Getenv("HOME") + "/.gomatrix-log"
	}
	logfile, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		fmt.Fprintf(output, "Could not open logfile. %s\n", err)
		return 1
	}
	defer logfile.Close()
	oldLogOutput := log.Writer()
	log.SetOutput(logfile)
	defer log.SetOutput(oldLogOutput) // restore

	waitg := &sync.WaitGroup{}
	ret := run(opts, output, waitg)

	fmt.Fprintln(output, "Thank you for connecting with Morpheus' Matrix API v4.2. Have a nice day!")

	waitg.Wait()

	return ret
}

func run(opts Opts, output io.Writer, waitg *sync.WaitGroup) int {
	if opts.FPS < 1 || opts.FPS > 60 {
		fmt.Fprintln(output, "Error: option --fps not within range 1-60")
		return 1
	}
	// Start profiling (if required)
	if len(opts.Profile) > 0 {
		f, err := os.Create(opts.Profile)
		if err != nil {
			fmt.Fprintf(output, "Error opening profiling file: %s\n", err)
			return 1
		}
		err = pprof.StartCPUProfile(f)
		if err != nil {
			fmt.Fprintf(output, "Error start profiling : %s\n", err)
			return 1
		}
		defer func() {
			// stop profiling (if required)
			if len(opts.Profile) > 0 {
				pprof.StopCPUProfile()
			}
		}()
	}
	// Use a println for fun..
	fmt.Fprintln(output, "Opening connection to The Matrix.. Please stand by..")
	log.Println("-------------")
	log.Println("Starting gomatrix. This logfile is for development/debug purposes.")
	if opts.Ascii {
		characters = alphaNumerics
	} else if opts.Kana {
		characters = halfWidthKana
	} else {
		characters = allTheCharacters
	}

	if screen == nil {
		// initialize tcell
		var err error
		if screen, err = tcell.NewScreen(); err != nil {
			fmt.Fprintln(output, "Could not start tcell for gomatrix. View ~/.gomatrix-log for error messages.")
			log.Printf("Cannot alloc screen, tcell.NewScreen() gave an error:\n%s", err)
			return 1
		}

		err = screen.Init()
		if err != nil {
			fmt.Fprintln(output, "Could not start tcell for gomatrix. View ~/.gomatrix-log for error messages.")
			log.Printf("Cannot start gomatrix, Screen.Init() gave an error:\n%s", err)
			return 1
		}

		defer func() {
			// close down
			screen.Fini()
		}()
	}

	screen.HideCursor()
	screen.SetStyle(tcell.StyleDefault.
		Background(tcell.ColorBlack).
		Foreground(tcell.ColorBlack))
	screen.Clear()

	stopCh := make(chan struct{})
	defer close(stopCh)

	// channel used to notify StreamDisplayManager
	var sizesUpdateCh = make(chan sizes)
	defer close(sizesUpdateCh)

	// streamDisplays by column number
	var streamDisplaysByColumn = make(map[int]*StreamDisplay)

	// StreamDisplay manager
	waitg.Add(1)
	go func() {
		defer waitg.Done()
		var lastWidth int

		for newSizes := range sizesUpdateCh {
			log.Printf("New width: %d\n", newSizes.width)
			diffWidth := newSizes.width - lastWidth

			if diffWidth == 0 {
				// same column size, wait for new information
				log.Println("Got resize over channel, but diffWidth = 0")
				continue
			}

			if diffWidth > 0 {
				log.Printf("Starting %d new SD's\n", diffWidth)
				for newColumn := lastWidth; newColumn < newSizes.width; newColumn++ {
					// create stream display
					sd := &StreamDisplay{
						column:    newColumn,
						stopCh:    make(chan struct{}),
						streams:   make(map[*Stream]bool),
						newStream: make(chan bool, 1), // will only be filled at start and when a spawning stream has it's tail released
					}
					streamDisplaysByColumn[newColumn] = sd

					// start StreamDisplay in goroutine
					waitg.Add(1)
					go func() {
						defer waitg.Done()
						sd.run()
					}()

					// create first new stream
					sd.newStream <- true
				}
				lastWidth = newSizes.width
			}

			if diffWidth < 0 {
				log.Printf("Closing %d SD's\n", diffWidth)
				for closeColumn := lastWidth - 1; closeColumn > newSizes.width; closeColumn-- {
					// get sd
					sd := streamDisplaysByColumn[closeColumn]

					// delete from map
					delete(streamDisplaysByColumn, closeColumn)

					// inform sd that it's being closed
					close(sd.stopCh)
				}
				lastWidth = newSizes.width
			}
		}

		// Cleanup all streams:
		log.Printf("Closing %d SD's\n", lastWidth)
		for closeColumn := 0; closeColumn < lastWidth; closeColumn++ {
			// get sd
			sd := streamDisplaysByColumn[closeColumn]

			// delete from map
			delete(streamDisplaysByColumn, closeColumn)

			// inform sd that it's being closed
			close(sd.stopCh)
		}

	}()

	// set initial sizes
	gLock.Lock()
	curSizes.setSizes(screen.Size())
	gLock.Unlock()
	sizesUpdateCh <- curSizes

	// flusher flushes the termbox every x milliseconds
	curFPS := opts.FPS
	fpsSleepTime := time.Duration(1000000/curFPS) * time.Microsecond
	fmt.Fprintf(output, "fps sleep time: %s\n", fpsSleepTime.String())
	waitg.Add(1)
	go func() {
		defer waitg.Done()
		for {
			time.Sleep(fpsSleepTime)
			select {
			case <-stopCh:
				return
			default:
				screen.Show()
			}
		}
	}()

	// make chan for tembox events and run poller to send events on chan
	eventChan := make(chan tcell.Event)
	waitg.Add(1)
	go func() {
		defer waitg.Done()
		for {
			event := screen.PollEvent()
			if event == nil {
				break
			}
			eventChan <- event
		}
		close(eventChan)
	}()

	// register signals to channel
	sigChan := make(chan os.Signal)
	signal.Notify(sigChan, os.Interrupt, os.Kill)
	defer signal.Stop(sigChan)

	// handle tcell events and unix signals
EVENTS:
	for {
		// select for either event or signal
		select {
		case event := <-eventChan:
			if event == nil {
				break
			}
			log.Printf("Have event: \n%s", spew.Sdump(event))
			// switch on event type
			switch ev := event.(type) {
			case *tcell.EventKey:
				switch ev.Key() {
				case tcell.KeyCtrlZ, tcell.KeyCtrlC:
					break EVENTS

				case tcell.KeyCtrlL:
					screen.Sync()

				case tcell.KeyRune:
					switch ev.Rune() {
					case 'q':
						break EVENTS

					case 'c':
						screen.Clear()

					case 'k':
						characters = halfWidthKana

					case 'b': // "both"
						characters = allTheCharacters

					case '+': // speed it up
						if curFPS < 60 {
							curFPS++
							fpsSleepTime = time.Duration(1000000/curFPS) * time.Microsecond
						}

					case '-': // slow it down
						if curFPS > 1 {
							curFPS--
							fpsSleepTime = time.Duration(1000000/curFPS) * time.Microsecond
						}

					case '=': // set the speed back to where it started
						curFPS = opts.FPS
						fpsSleepTime = time.Duration(1000000/curFPS) * time.Microsecond
					}

					//++ TODO: add more fun keys (slowmo? freeze? rampage?)
				}
			case *tcell.EventResize: // set sizes
				w, h := ev.Size()
				gLock.Lock()
				curSizes.setSizes(w, h)
				gLock.Unlock()
				sizesUpdateCh <- curSizes
			case *tcell.EventError: // quit
				log.Panicf("Quitting because of tcell error: %v", ev.Error())
			}

		case signal := <-sigChan:
			log.Printf("Have signal: \n%s", spew.Sdump(signal))
			break EVENTS
		}
	}

	log.Println("stopping gomatrix")

	return 0
}
