package agent

type InputCreator interface {
	New(config *AgentConfig) *Streamer
}

// A Streamer takes input based on its config and type and streams to
// the appropriate streams channel. A 1 on the streams.quit channel
// indicates that the Streamer should gracefully quit and return a 1 on
// the streams.quit channel when completed.
type Streamer interface {
	Stream(streams *Streams) (err error)
}
