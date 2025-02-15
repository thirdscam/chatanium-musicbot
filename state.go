package main

import (
	"errors"
	"sync"
	"time"

	Provider "github.com/thirdscam/chatanium-musicbot/provider"
)

type ChannelID string

// The state of the music for each channel
var states map[ChannelID]*State = make(map[ChannelID]*State)

var (
	errEmptyQueue            = errors.New("queue is empty")
	errIndexOutOfRange       = errors.New("index is out of range")
	errIndexCannotBeNegative = errors.New("index cannot be negative")
	errSignalTimeout         = errors.New("signal timeout")
)

func GetState(channelID ChannelID) *State {
	if _, exists := states[channelID]; exists {
		return states[channelID]
	}

	states[channelID] = &State{
		queue:     []Provider.Music{},
		isPlaying: false,
		loop:      false,
		pause:     make(chan bool),
		skip:      make(chan bool),
	}

	return states[channelID]
}

// States are created per channel and are a kind of multifunctional queue, with a focus on music queues.
// It can also be locked if necessary with an RWMutex.
type State struct {
	sync.RWMutex
	queue     []Provider.Music
	isPlaying bool
	loop      bool
	pause     chan bool
	skip      chan bool
}

func (s *State) GetQueue() []Provider.Music {
	s.RLock()
	defer s.RUnlock()

	return s.queue
}

func (s *State) IsQueueEmpty() bool {
	s.RLock()
	defer s.RUnlock()

	return len(s.queue) == 0
}

// enqueues a music to the queue
func (s *State) Enqueue(music ...Provider.Music) error {
	s.Lock()
	defer s.Unlock()

	s.queue = append(s.queue, music...)
	return nil
}

func (s *State) Dequeue() error {
	s.Lock()
	defer s.Unlock()

	if len(s.queue) == 0 {
		return errEmptyQueue
	}

	s.queue = s.queue[1:]
	return nil
}

func (s *State) Remove(index int) (Provider.Music, error) {
	s.Lock()
	defer s.Unlock()

	if index < 0 {
		return Provider.Music{}, errIndexCannotBeNegative
	}

	if len(s.queue) == 0 {
		return Provider.Music{}, errEmptyQueue
	}

	if index >= len(s.queue) {
		return Provider.Music{}, errIndexOutOfRange
	}

	// Remove the music at the specified index
	target := s.queue[index]
	s.queue = append(s.queue[:index], s.queue[index+1:]...)

	return target, nil
}

func (s *State) Insert(index int, music ...Provider.Music) error {
	s.Lock()
	defer s.Unlock()

	if index < 0 {
		return errIndexCannotBeNegative
	}

	if index > len(s.queue) {
		return errIndexOutOfRange
	}

	// Insert the music at the specified index
	s.queue = append(s.queue[:index], append(music, s.queue[index:]...)...)
	return nil
}

func (s *State) ToggleLoop() bool {
	s.Lock()
	defer s.Unlock()

	s.loop = !s.loop
	return s.loop
}

func (s *State) IsLoopMode() bool {
	return s.loop
}

// Get the first music in the queue
func (s *State) GetFront() Provider.Music {
	if len(s.queue) == 0 {
		return Provider.Music{}
	}

	return s.queue[0]
}

func (s *State) SetIsPlaying(isRunning bool) {
	s.Lock()
	defer s.Unlock()

	s.isPlaying = isRunning
}

func (s *State) IsPlaying() bool {
	s.RLock()
	defer s.RUnlock()

	return s.isPlaying
}

// Get the first music in the queue and remove it from the queue
func (s *State) Pop() Provider.Music {
	if len(s.queue) == 0 {
		return Provider.Music{}
	}

	// Remove the first element from the queue
	target := s.queue[0]
	s.queue = s.queue[1:]

	return target
}

// Get the pause and skip signals
func (s *State) GetSignals() (chan bool, chan bool) {
	return s.pause, s.skip
}

// Check if the queue contains the music
func (s *State) IsExistMusic(music Provider.Music) bool {
	s.Lock()
	defer s.Unlock()

	for _, m := range s.queue {
		if m.Id == music.Id {
			return true
		}
	}

	return false
}

// Pause the music (send a pause signal to the music player thread)
func (s *State) Pause() error {
	s.Lock()
	defer s.Unlock()

	if len(s.queue) == 0 {
		return errEmptyQueue
	}

	// Music player control thread
	done := make(chan bool)
	go func() {
		// Send pause/resume signal to the music player thread
		s.pause <- true

		// Wait for the music player control finishes
		done <- true
	}()

	select {
	case <-time.After(3 * time.Second):
		// if the music player control thread doesn't finish in 3 seconds
		return errSignalTimeout

	case <-done:
		// if the music player control thread finishes (successfully paused/resumed)
		return nil
	}
}

// Skip the music (send a skip signal to the music player thread)
func (s *State) Skip() error {
	s.Lock()
	defer s.Unlock()

	if len(s.queue) == 0 {
		return errEmptyQueue
	}

	// Music player control thread
	done := make(chan bool)
	go func() {
		// Send skip signal to the music player thread
		s.skip <- true

		// Wait for the music player control finishes
		done <- true
	}()

	select {
	case <-time.After(3 * time.Second):
		// if the music player control thread doesn't finish in 3 seconds
		return errSignalTimeout

	case <-done:
		// if the music player control thread finishes (successfully paused/resumed)
		return nil
	}
}
