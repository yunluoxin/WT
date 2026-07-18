package tui

import (
	"os"
	"time"

	"golang.org/x/term"
)

type key int

const (
	keyUnknown key = iota
	keyUp
	keyDown
	keyEnter
	keyEsc
	keyCtrlC
	keyQuit
)

// readByte reads one byte, honoring the pushback buffer (Windows peek).
func readByte(fd int) (byte, error) {
	b, err := readByteImpl(fd)
	return b, err
}

// readKey reads one keypress, disambiguating a bare Escape from escape
// sequences via a short peek timeout.
func readKey(fd int) (key, int) {
	first, err := readByte(fd)
	if err != nil {
		return keyCtrlC, -1
	}
	buf := []byte{first}
	if _, err := os.Stdin.Read(buf); err != nil {
		return keyCtrlC, -1
	}
	ch := buf[0]
	switch ch {
	case 0x1b:
		if !peekReady(fd, 50*time.Millisecond) {
			return keyEsc, -1
		}
		seq := make([]byte, 2)
		n, _ := os.Stdin.Read(seq)
		if n >= 2 && seq[0] == '[' {
			switch seq[1] {
			case 'A':
				return keyUp, -1
			case 'B':
				return keyDown, -1
			}
		}
		// Windows legacy console prefix bytes
		if n >= 1 && (seq[0] == 0x00 || seq[0] == 0xe0) {
			n2, _ := os.Stdin.Read(seq[1:2])
			if n2 == 1 {
				switch seq[1] {
				case 'H':
					return keyUp, -1
				case 'P':
					return keyDown, -1
				}
			}
		}
		return keyUnknown, -1
	case '\r', '\n':
		return keyEnter, -1
	case 0x03:
		return keyCtrlC, -1
	case 'q':
		return keyQuit, -1
	}
	if ch >= '1' && ch <= '9' {
		return keyUnknown, int(ch - '1')
	}
	return keyUnknown, -1
}

// rawSelect runs the interactive selector with the terminal in raw mode.
func rawSelect(items []Item, title string, defaultIndex int) (string, bool) {
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return fallbackSelect(items, title, defaultIndex)
	}
	writeStderr("\x1b[?25l") // hide cursor
	restored := false
	restore := func() {
		if !restored {
			restored = true
			term.Restore(fd, oldState)
			writeStderr("\x1b[?25h") // show cursor
		}
	}
	defer restore()

	selected := defaultIndex
	totalLines := len(items) + 2
	render(items, title, selected, true)

	for {
		k, digit := readKey(fd)
		switch {
		case k == keyEnter:
			cleanup(totalLines)
			restore()
			return items[selected].Value, true
		case k == keyCtrlC || k == keyQuit || k == keyEsc:
			cleanup(totalLines)
			restore()
			return "", false
		case k == keyUp:
			selected = (selected - 1 + len(items)) % len(items)
			render(items, title, selected, false)
		case k == keyDown:
			selected = (selected + 1) % len(items)
			render(items, title, selected, false)
		case digit >= 0 && digit < len(items):
			cleanup(totalLines)
			restore()
			return items[digit].Value, true
		}
	}
}
