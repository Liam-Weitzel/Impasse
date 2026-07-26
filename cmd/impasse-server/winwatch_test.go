package main

import (
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/gliderlabs/ssh"
)

// waitWin takes the next window from ch, or fails.
func waitWin(t *testing.T, ch chan ssh.Window, what string) ssh.Window {
	t.Helper()
	select {
	case win := <-ch:
		return win
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
		return ssh.Window{}
	}
}

// A new owner is given the current size without waiting for a resize, so the
// renderer starts at the size the terminal already is.
func TestWinWatchHandsCurrentSizeToNewOwner(t *testing.T) {
	src := make(chan ssh.Window)
	w := newWinWatch(ssh.Window{Width: 80, Height: 24}, src)

	got := make(chan ssh.Window, 4)
	w.watch(func(win ssh.Window) { got <- win })

	if win := waitWin(t, got, "initial size"); win.Width != 80 || win.Height != 24 {
		t.Fatalf("got %dx%d, want 80x24", win.Width, win.Height)
	}
}

// Only the current owner sees resizes. Two owners reading the same channel is
// what made the renderer miss about half of them.
func TestWinWatchOnlyCurrentOwnerGetsResizes(t *testing.T) {
	src := make(chan ssh.Window)
	w := newWinWatch(ssh.Window{Width: 80, Height: 24}, src)

	first := make(chan ssh.Window, 8)
	w.watch(func(win ssh.Window) { first <- win })
	waitWin(t, first, "first owner's initial size")

	src <- ssh.Window{Width: 100, Height: 40}
	if win := waitWin(t, first, "first owner's resize"); win.Width != 100 {
		t.Fatalf("got width %d, want 100", win.Width)
	}

	// Hand over, the way the menu does when it gives the screen to the
	// renderer.
	second := make(chan ssh.Window, 8)
	w.watch(func(win ssh.Window) { second <- win })

	// The new owner is caught up to the last size seen, not the one the
	// session started with.
	if win := waitWin(t, second, "second owner's initial size"); win.Width != 100 {
		t.Fatalf("got width %d, want 100", win.Width)
	}

	src <- ssh.Window{Width: 120, Height: 50}
	if win := waitWin(t, second, "second owner's resize"); win.Width != 120 {
		t.Fatalf("got width %d, want 120", win.Width)
	}

	select {
	case win := <-first:
		t.Fatalf("replaced owner still got %dx%d", win.Width, win.Height)
	default:
	}
}

// watch must not run the owner on the caller's goroutine. bubbletea's Send
// blocks until its Run loop starts reading, so calling it inline before Run
// deadlocks the whole session and the player sees ssh hang.
func TestWinWatchDoesNotBlockCaller(t *testing.T) {
	src := make(chan ssh.Window)
	w := newWinWatch(ssh.Window{Width: 80, Height: 24}, src)

	release := make(chan struct{})
	entered := make(chan struct{}, 1)

	returned := make(chan struct{})
	go func() {
		defer close(returned)
		w.watch(func(ssh.Window) {
			select {
			case entered <- struct{}{}:
			default:
			}
			<-release
		})
	}()

	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("watch blocked on an owner that has not finished")
	}

	waitWin := time.After(2 * time.Second)
	select {
	case <-entered:
	case <-waitWin:
		t.Fatal("owner was never called")
	}

	close(release)
}

// Keystrokes go to the current owner only. Two readers on one session would
// each take about half of them, so the player's typing would half disappear as
// they moved between the menu and the game.
func TestSessionInputGoesToCurrentOwnerOnly(t *testing.T) {
	var si sessionInput

	var menu, game bytes.Buffer

	si.to(&menu)
	if _, err := si.Write([]byte("play")); err != nil {
		t.Fatalf("write: %v", err)
	}

	si.to(&game)
	if _, err := si.Write([]byte("wasd")); err != nil {
		t.Fatalf("write: %v", err)
	}

	if menu.String() != "play" {
		t.Errorf("menu got %q, want %q", menu.String(), "play")
	}
	if game.String() != "wasd" {
		t.Errorf("game got %q, want %q", game.String(), "wasd")
	}
}

// With nobody watching, input is dropped rather than blocking or failing. It
// was typed at a screen that has gone.
func TestSessionInputWithNoOwnerIsDropped(t *testing.T) {
	var si sessionInput

	n, err := si.Write([]byte("hello"))
	if err != nil || n != 5 {
		t.Fatalf("write with no owner: n %d err %v", n, err)
	}
}

// A renderer exiting closes its pty while a keystroke may be in flight. That
// must not stop the session's one copy, or the player loses input for good.
func TestSessionInputSurvivesAFailingOwner(t *testing.T) {
	var si sessionInput

	pr, pw := io.Pipe()
	pr.Close() // every write to pw now fails

	si.to(pw)
	if n, err := si.Write([]byte("lost")); err != nil || n != 4 {
		t.Fatalf("write to a dead owner: n %d err %v", n, err)
	}

	var next bytes.Buffer
	si.to(&next)
	if _, err := si.Write([]byte("kept")); err != nil {
		t.Fatalf("write after a dead owner: %v", err)
	}
	if next.String() != "kept" {
		t.Errorf("got %q, want the next owner still receiving", next.String())
	}
}
