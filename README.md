# Team Information
- **Name:** R Yeeshu Dhurandhar, Abhiram Gottumukkala
- **AndrewID:** rdhurand, agottumu
- **GitHub user IDs:** ryeeshu, agottumu-lgtm
- **GitHub Classroom team name:** split_brain
- **Work distribution:** We usually sit together and work through the code using pair programming. For smaller sections that one of us completes in the absence of the other, we first catch up and review the changes together before continuing in the next session. The only part we worked on individually was the documentation, which we divided equally between us.

# Lab 1 Summary (Remote Objects / RPC Library)

In this lab, we built a small remote objects library in Go, similar to the idea of RPC/RMI. The goal is that a client can call methods on a remote server as if they are normal local function calls, while the library handles the networking details in the background.

The library has two main parts:

- CalleeStub (server side): runs a multi-threaded TCP server, receives requests, checks the method name and argument types, calls the real method on the server-side object using reflection, and sends back the return values.

- CallerStubCreator (client side): takes a “service interface” struct (where each field is a function prototype) and fills in those function fields using reflect.MakeFunc, so calling the function automatically performs a remote call.

To make things more realistic, the lab also includes a LeakySocket wrapper that can randomly drop or delay messages, so the library has to handle unreliable communication.

After completing the library, we also wrote a simple example application (client + server) that uses the library to interact with a stateful object on the server, including methods that pass and return more complex Go types like structs, slices, and maps.



# Implementation Details

We implemented the remote library by building both sides of the interaction: a server-side stub (callee) that hosts a real object and executes methods on it, and a client-side stub (caller) that makes remote calls look like normal function calls.

On the callee side, we created a CalleeStub that starts a TCP listener and runs an accept loop. Every incoming connection is handled in its own goroutine so multiple clients can call methods at the same time. For each request, we receive a RequestMsg, decode it using gob, validate that the requested method exists in the service interface, and check that the argument count and types match. Then we use reflection (MethodByName + Call) to invoke the actual method on the server-side object. The return values (except the final RemoteError) are packed into a ReplyMsg, encoded, and sent back to the caller. We also track whether the server is running and maintain a call counter across restarts.

On the caller side, we implemented CallerStubCreator using reflection. We iterate over every function field in the provided service interface struct and replace it with a real function using reflect.MakeFunc. Each generated function converts its input arguments into []any, performs a network call to the server, decodes the reply, and converts the returned values back into the expected return types. If any network/protocol issue happens, we return zero values for normal returns and set the final RemoteError with a useful message.

For communication, we used the provided LeakySocket wrapper on both ends. Since it can drop or delay messages, our caller side retries remote calls (within a deadline) when a request or reply fails. We used gob encoding/decoding for request and reply messages, and for application structs we made sure the types are registered on both the client and server so complex values can be transported correctly.

Finally, we wrote a small example application with separate server and client programs. The server hosts a stateful object, and the client calls multiple methods (including ones that mutate state) using the same service interface struct, which demonstrates that multiple clients can connect and observe shared state through the remote calls.



# Summary of Error and Failure Cases

### Failures our remote library handles

- **Invalid inputs when building stubs**
  - `NewCalleeStub` and `CallerStubCreator` reject `nil` inputs and reject “non-remote” interfaces (for example, fields that are not functions, or functions that do not end with `RemoteError`).
- **Server not running / connection refused**
  - If the callee is stopped or the address is wrong, the caller fails the call and returns a non-empty `RemoteError` instead of hanging.
- **Unknown method name**
  - If the caller requests a method that is not in the callee’s service interface, the callee returns a failure reply (`RemoteError` set).
- **Argument count mismatch**
  - If the caller sends the wrong number of arguments, the callee detects it and returns a failure reply.
- **Argument type mismatch**
  - The callee checks that each argument is assignable/convertible to the expected method parameter type. If not, it returns a failure reply.
- **Message loss / delay (LeakySocket)**
  - The caller retries remote calls within a deadline when a request or reply is dropped or times out, instead of failing immediately.
- **Decode/encode failures**
  - If gob decoding or encoding fails (corrupted bytes, wrong format), the call returns a failure `RemoteError` instead of crashing.
- **Concurrency on the server**
  - Each connection is handled in a goroutine and stateful application objects use locks, so multiple clients can call methods at the same time without data races.

### Failures that can still cause the application to fail (limitations)

- **Gob type registration requirements (complex types)**
  - Our protocol uses `any` (`interface{}`) inside request/reply messages. For gob, that means any concrete types carried inside those `any` values must be registered consistently on both client and server. If a type is not registered (for example, `Task` in our app), gob will fail at runtime and the call will return a `RemoteError`. This is easy to miss when adding new structs or maps.
- **Large messages**
  - The current receive logic uses a fixed-size buffer (4096 bytes). If a request or reply grows larger than this (large maps, large slices, long strings), it can get truncated and decoding can fail. This is not covered by the tests and would require a length-prefixed framing protocol to fully fix.
- **Partial sends / message framing**
  - The implementation assumes one gob-encoded message corresponds to one read/write cycle. TCP does not guarantee message boundaries, so in extreme cases a message could arrive in multiple chunks or multiple messages could be coalesced. The lab tests are small enough that this usually works, but it is not a fully robust framing design.
- **Timeout tuning**
  - Our retry and timeout values are fixed. If the machine is very slow or heavily loaded, a valid reply might arrive after our deadline and the client would fail even though the server eventually computed the result.
- **Exactly-once vs at-least-once behavior**
  - With retries, a request can be executed more than once if the caller does not receive the reply (e.g., reply dropped after the server already executed the method). This is fine for read-only methods, but for state-changing methods it can cause duplicated side effects unless the application is designed to be idempotent or uses request IDs/deduplication (we did not implement deduplication).
- **Server-side panics inside application methods**
  - If the hosted object’s method panics, it can crash the goroutine handling that connection and the client may see a failure or hang depending on when it happened. We did not add panic recovery around method invocation.
- **Unsupported parameter/return types**
  - We assume all arguments and return values are gob-serializable. Types with channels, functions, or other non-serializable fields will break at runtime.
- **Method signature assumptions**
  - We assume the last return value is always `RemoteError`, and we treat that as the “library/protocol error channel.” Application errors are expected to be represented separately (for example, string messages). If someone designs a service interface that encodes application errors differently, the behavior might not match their expectation.

### Application-level failure scenarios (example app)

- **Bad client input**
  - Empty owner/title, invalid IDs, or invalid status strings can trigger application-level errors (returned as strings). These are not `RemoteError`s, but they still represent failure from the user’s perspective.
- **Multiple clients and shared state**
  - The server object is stateful and shared across all clients on the same port. If the server is restarted, state is lost (in-memory only). Clients will see missing tasks/empty state after restart.
- **Using different ports**
  - Clients connecting to different ports will talk to different instances (expected), but users might think they are connecting to the “same server” and get confused if the address is inconsistent.


# Application User’s Guide (myapp)
(Please see `application_users_guide.md` for a detailed user guide for the example application, including what it does, how to run it, and design notes.)

# References:
We used following references while writing the code:
- https://pkg.go.dev/net/rpc
- https://go.dev/blog/laws-of-reflection
- Go documentation: https://go.dev/doc/
- For Debugging: https://github.com/go-delve/delve
- Class slides, especially, reflection topic

Use of AI tools:
- To understand the test cases.
- To understand the error messages and how to debug them.
- To learn some Go language syntax and semantics used in this lab.
- To get syntax help for go language, e.g., how to use mutexes, how to read from a TCP connection, how to write to a TCP connection, etc.
- To get a proper structure of how the comments should be written so that the gomarkdoc generates clean documentation. E.g., what should be included in the overview section, how to write function comments, etc.
- To paraphrase certain sections of the README.md file and comments in the code files.
- To convert normal text to markdown format.



