package transport

// Message is a buffer and size sent/received by a connection.
type Message struct {
	Buffer []byte
	Size   int
}
