// Package main implements the TaskService client application for Lab 1.
//
// This client demonstrates how to use the custom `remote` library (RMI-style RPC)
// by constructing a caller stub from a service interface struct, then invoking
// multiple remote methods that exercise:
//
//   - Primitive types (string, int)
//   - Complex types (struct Task, maps, slices)
//   - Stateful behavior (creating tasks, updating status, and observing shared state)
//
// The client is intended to be run multiple times (possibly with different owners)
// to show that state persists on the server and is visible across independent clients.
package main

import (
	"encoding/gob"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"time"

	"remote"
)

// DefaultAddr is the default server address used when -addr is not provided.
const DefaultAddr = "localhost:19000"

// Task is the primary stateful object manipulated by the example application.
//
// The Task struct is intentionally non-trivial (it includes a slice and a time.Time)
// so the example demonstrates gob-serializing richer data structures across the network.
type Task struct {
	ID        int
	Owner     string
	Title     string
	Tags      []string
	Status    string
	CreatedAt time.Time
}

// init registers application-level types with gob so they can be encoded/decoded when
// carried inside interface{} (any) fields in RequestMsg/ReplyMsg.
//
// Because this application sends/receives Task and maps containing Task through the
// remote library, both the client and server must register the same types.
//
// Note: This registration is required because RequestMsg.Args and ReplyMsg.Reply are
// []any; gob needs explicit type registration for concrete types stored in interface{}.
func init() {
	// Must match server registrations for types carried inside interface{}.
	gob.Register(Task{})
	gob.Register(map[int]Task{})
	gob.Register(map[string]int{})
	gob.Register([]string{})
}

// TaskService is the service interface (stub definition) shared by both client and server.
//
// Each field is a function prototype. The remote library replaces these nil function
// fields with generated implementations (via reflection) when CallerStubCreator is called.
//
// Requirement reminder: the last return value of each method is remote.RemoteError,
// which represents stub/protocol/network failures (not application-level errors).
type TaskService struct {
	Health       func() (string, remote.RemoteError)
	AddTask      func(Task) (int, string, remote.RemoteError)
	GetTask      func(int) (Task, string, remote.RemoteError)
	ListByOwner  func(string) (map[int]Task, remote.RemoteError)
	UpdateStatus func(int, string) (string, remote.RemoteError)
	Stats        func() (map[string]int, remote.RemoteError)
}

// randName creates a pseudo-random string of length n.
//
// This is used to generate a unique client identifier (and optionally a default owner)
// to make repeated client runs easy to distinguish in output.
//
// Note: This matches the style used in the class example (seed RNG each call).
func randName(n int) string {
	rand.Seed(time.Now().UnixNano())
	letters := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

// main is the entrypoint for the example client.
//
// It performs the following workflow:
//  1. Parse -addr and -owner flags
//  2. Create a TaskService stub using remote.CallerStubCreator
//  3. Call several remote methods to demonstrate:
//     - Connectivity and health check
//     - Stateful creation of a Task (AddTask)
//     - Fetching a Task back (GetTask)
//     - Mutating state (UpdateStatus)
//     - Querying state via complex return types (ListByOwner, Stats)
//
// The program exits with a non-zero status on any RemoteError or application-level error.
func main() {
	addr := flag.String("addr", DefaultAddr, "Address ('ip:port') of remote host")
	owner := flag.String("owner", "", "Owner name (used to demonstrate shared state across clients)")
	flag.Parse()

	if *owner == "" {
		// Default owner so it still runs easily without specifying -owner.
		o := "user-" + randName(4)
		owner = &o
	}

	clientName := randName(8)
	fmt.Println("Starting client", clientName, "-> addr:", *addr, "owner:", *owner)

	// Create the caller stub by populating the TaskService function fields using reflection.
	// After this call succeeds, svc.<Method>(...) performs real remote calls.
	svc := &TaskService{}
	if err := remote.CallerStubCreator(svc, *addr, true, true); err != nil {
		log.Fatal("CallerStubCreator failed: " + err.Error())
	}

	// 1) Health check: simple method returning a string.
	h, re := svc.Health()
	if re.Error() != "" {
		log.Fatal("Health failed: " + re.Error())
	}
	fmt.Println("Health:", h)

	// 2) Add a task: send a complex struct + slice to the server.
	newTask := Task{
		Owner: *owner,
		Title: "demo task from " + clientName,
		Tags:  []string{"demo", "rpc", "lab1"},
	}
	id, appErr, re := svc.AddTask(newTask)
	if re.Error() != "" {
		log.Fatal("AddTask RemoteError: " + re.Error())
	}
	if appErr != "" {
		log.Fatal("AddTask app error: " + appErr)
	}
	fmt.Println("Added task id:", id)

	// 3) Get the task back: proves round-trip serialization/deserialization of Task.
	got, appErr, re := svc.GetTask(id)
	if re.Error() != "" {
		log.Fatal("GetTask RemoteError: " + re.Error())
	}
	if appErr != "" {
		log.Fatal("GetTask app error: " + appErr)
	}
	fmt.Printf("Fetched task: id=%d owner=%q title=%q status=%q tags=%v\n",
		got.ID, got.Owner, got.Title, got.Status, got.Tags)

	// 4) Update status: demonstrates state mutation on the server.
	appErr2, re := svc.UpdateStatus(id, "done")
	if re.Error() != "" {
		log.Fatal("UpdateStatus RemoteError: " + re.Error())
	}
	if appErr2 != "" {
		log.Fatal("UpdateStatus app error: " + appErr2)
	}
	fmt.Println("Updated status -> done")

	// 5) List tasks for this owner: demonstrates returning a complex map[int]Task.
	m, re := svc.ListByOwner(*owner)
	if re.Error() != "" {
		log.Fatal("ListByOwner failed: " + re.Error())
	}
	fmt.Printf("Tasks for owner %q:\n", *owner)
	if len(m) == 0 {
		fmt.Println("  (none)")
	} else {
		for tid, t := range m {
			fmt.Printf("  - id=%d title=%q status=%q tags=%v\n", tid, t.Title, t.Status, t.Tags)
		}
	}

	// 6) Global stats: returns map[string]int, used to show global state across all owners.
	stats, re := svc.Stats()
	if re.Error() != "" {
		log.Fatal("Stats failed: " + re.Error())
	}
	fmt.Println("Global status counts:", stats)

	fmt.Println("Client done.")
}
