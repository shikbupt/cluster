package cluster

type Event interface{}

type memberChangeEvent struct {
	IP []string
}

type MemberJoinEvent struct {
	memberChangeEvent
}

type MemberLeftEvent struct {
	memberChangeEvent
}

type MemberRemoveEvent struct {
	memberChangeEvent
}

type BroadcastEvent struct {
	Name    string
	Payload []byte
}

type Members struct {
	IP []string
}
