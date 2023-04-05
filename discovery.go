package cluster

import "github.com/hashicorp/serf/serf"

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

type QueryEvent struct {
	*serf.Query
}

type Members struct {
	IP []string
}
