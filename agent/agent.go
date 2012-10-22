package agent

type Streams struct {
	json chan *jsonMessage
	stat chan *Packet
	quit chan int
}
