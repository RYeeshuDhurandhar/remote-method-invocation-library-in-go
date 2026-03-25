// Package main implements the TaskService server application for Lab 1.
//
// This program hosts a stateful TaskBoard object behind a CalleeStub created
// from the custom `remote` library. Clients interact with the TaskBoard via a
// caller stub built from the same TaskService interface.
//
// The server demonstrates:
//   - A stateful remote object (TaskBoard) shared by all clients connected to the same TCP port
//   - Multiple methods (Health, AddTask, GetTask, ListByOwner, UpdateStatus, Stats)
//   - Primitive and complex data types (struct Task, slices, maps, time.Time)
//   - Concurrency protection on shared state (sync.Mutex)
//   - Graceful shutdown via OS signals (SIGINT/SIGTERM)
package main

import (
	"encoding/gob"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"remote"
)

// DefaultAddr is the default TCP address the server listens on if -addr is not provided.
const DefaultAddr = "localhost:19000"

// Task is a complex application-level type that can be sent/received via the remote library.
//
// Because the remote library transports arguments/results inside interface{} (any) fields
// (e.g., RequestMsg.Args and ReplyMsg.Reply), Task must be registered with gob on both the
// client and server, or encoding/decoding will fail at runtime.
type Task struct {
	ID        int
	Owner     string
	Title     string
	Tags      []string
	Status    string
	CreatedAt time.Time
}

// init registers all application-level concrete types that may appear inside interface{}.
//
// This must mirror the client-side registrations, because gob requires explicit registration
// for concrete values stored in interface{} during encoding/decoding.
func init() {
	// Register all concrete types that might appear inside []any / interface{}.
	gob.Register(Task{})
	gob.Register(map[int]Task{})
	gob.Register(map[string]int{})
	gob.Register([]string{})
}

// TaskService is the service interface shared by caller and callee.
//
// The remote library uses this struct type as a service description:
//   - On the server side, NewCalleeStub uses TaskService{} to validate method signatures.
//   - On the client side, CallerStubCreator fills in these function fields to form a stub.
//
// App-level errors are returned as strings (e.g., validation failures).
// remote.RemoteError is reserved for library/protocol/network errors.
type TaskService struct {
	Health       func() (string, remote.RemoteError)
	AddTask      func(Task) (int, string, remote.RemoteError)
	GetTask      func(int) (Task, string, remote.RemoteError)
	ListByOwner  func(string) (map[int]Task, remote.RemoteError)
	UpdateStatus func(int, string) (string, remote.RemoteError)
	Stats        func() (map[string]int, remote.RemoteError)
}

// TaskBoard is the server-side, stateful object that implements the TaskService behavior.
//
// All methods that access shared state must hold mu to remain safe under concurrent calls
// from multiple clients.
type TaskBoard struct {
	mu     sync.Mutex
	nextID int
	tasks  map[int]Task
}

// NewTaskBoard constructs an initialized TaskBoard instance with empty state.
func NewTaskBoard() *TaskBoard {
	return &TaskBoard{
		nextID: 1,
		tasks:  make(map[int]Task),
	}
}

// Health is a lightweight liveness check method.
func (tb *TaskBoard) Health() (string, remote.RemoteError) {
	fmt.Println("Health() called on server")
	return "ok", remote.RemoteError{}
}

// AddTask creates a new task and stores it in the TaskBoard.
//
// Returns:
//   - id: newly assigned task ID (positive integer)
//   - appErr: non-empty string if the request is invalid
//   - RemoteError: empty unless the remote library encountered a stub-level failure
func (tb *TaskBoard) AddTask(t Task) (int, string, remote.RemoteError) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	fmt.Printf("AddTask(owner=%q, title=%q) called on server\n", t.Owner, t.Title)

	// Application-level validation errors are communicated using the string return,
	// not remote.RemoteError.
	if t.Owner == "" {
		return 0, "owner cannot be empty", remote.RemoteError{}
	}
	if t.Title == "" {
		return 0, "title cannot be empty", remote.RemoteError{}
	}

	// Assign an ID and fill default values.
	id := tb.nextID
	tb.nextID++

	t.ID = id
	if t.Status == "" {
		t.Status = "open"
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now()
	}

	tb.tasks[id] = t
	return id, "", remote.RemoteError{}
}

// GetTask retrieves a task by ID.
//
// Returns:
//   - Task: the stored task if found
//   - appErr: non-empty string if the ID does not exist
//   - RemoteError: empty unless the remote library encountered a stub-level failure
func (tb *TaskBoard) GetTask(id int) (Task, string, remote.RemoteError) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	fmt.Printf("GetTask(id=%d) called on server\n", id)

	t, ok := tb.tasks[id]
	if !ok {
		return Task{}, fmt.Sprintf("no task with id %d", id), remote.RemoteError{}
	}
	return t, "", remote.RemoteError{}
}

// ListByOwner returns all tasks owned by a specific user.
//
// This demonstrates returning a complex type: map[int]Task.
func (tb *TaskBoard) ListByOwner(owner string) (map[int]Task, remote.RemoteError) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	fmt.Printf("ListByOwner(owner=%q) called on server\n", owner)

	out := make(map[int]Task)
	for id, t := range tb.tasks {
		if t.Owner == owner {
			out[id] = t
		}
	}
	return out, remote.RemoteError{}
}

// UpdateStatus mutates an existing task's status field.
//
// Returns an application-level error string if the task does not exist or the status is empty.
func (tb *TaskBoard) UpdateStatus(id int, status string) (string, remote.RemoteError) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	fmt.Printf("UpdateStatus(id=%d, status=%q) called on server\n", id, status)

	t, ok := tb.tasks[id]
	if !ok {
		return fmt.Sprintf("no task with id %d", id), remote.RemoteError{}
	}
	if status == "" {
		return "status cannot be empty", remote.RemoteError{}
	}

	t.Status = status
	tb.tasks[id] = t
	return "", remote.RemoteError{}
}

// Stats computes aggregate information about the TaskBoard.
//
// This demonstrates returning a complex type: map[string]int (counts by status).
func (tb *TaskBoard) Stats() (map[string]int, remote.RemoteError) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	fmt.Println("Stats() called on server")

	out := make(map[string]int)
	for _, t := range tb.tasks {
		out[t.Status]++
	}
	return out, remote.RemoteError{}
}

// main is the entrypoint for the server.
//
// It starts a CalleeStub bound to a TaskBoard instance, then waits for SIGINT/SIGTERM
// to stop gracefully.
func main() {
	addr := flag.String("addr", DefaultAddr, "Address ('ip:port') to listen on")
	flag.Parse()

	fmt.Println("Starting TaskBoard server on", *addr)

	board := NewTaskBoard()

	// Create a callee stub to expose TaskBoard methods over TCP at the provided address.
	callee, err := remote.NewCalleeStub(&TaskService{}, board, *addr, true, true)
	if err != nil {
		log.Fatal("NewCalleeStub failed: " + err.Error())
	}

	// Start listening and serving remote calls.
	if err := callee.Start(); err != nil {
		log.Fatal("Start failed: " + err.Error())
	}

	fmt.Println("Server is running. Press Ctrl+C to stop.")

	// Wait for an interrupt/terminate signal.
	exit := make(chan os.Signal, 1)
	signal.Notify(exit, syscall.SIGINT, syscall.SIGTERM)
	<-exit

	// Gracefully stop the callee stub (closes the listener).
	fmt.Println("\nStopping server...")
	if err := callee.Stop(); err != nil {
		log.Fatal("Stop failed: " + err.Error())
	}
	fmt.Println("Server stopped.")
}
