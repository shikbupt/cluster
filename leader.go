package cluster

import (
	"context"
	"sync"

	"github.com/hashicorp/serf/serf"
	"github.com/serialx/hashring"
)

type LeaderAction func(ctx context.Context,
	broadcast func(BroadcastEvent) error,
	query func(name string, payload []byte) (<-chan serf.NodeResponse, error)) error
type FollowerAction func(ctx context.Context,
	broadcastEvent <-chan BroadcastEvent,
	queryEvent <-chan QueryEvent) error

type Role string

const (
	Nobody   Role = "nobody"
	Leader   Role = "leader"
	Follower Role = "follower"
)

type Candidate struct {
	gossip         *GossipMembership
	nodeLock       sync.RWMutex
	hashRing       *hashring.HashRing
	watch          <-chan Event
	broadcastEvent chan BroadcastEvent
	queryEvent     chan QueryEvent

	leaderMayChange chan struct{}
	current         Role

	leaderAction   LeaderAction
	followerAction FollowerAction
	self           string // local ip
	peers          []string
}

func NewCandidate(selfip string, peers []string, leaderAction LeaderAction, followerAction FollowerAction) (*Candidate, error) {
	l := &Candidate{
		hashRing:        hashring.New([]string{}),
		leaderMayChange: make(chan struct{}),
		current:         Nobody,
	}
	l.leaderAction = leaderAction
	l.followerAction = followerAction

	l.self = selfip
	for _, peer := range peers {
		if peer == l.self {
			l.peers = peers
			return l, nil
		}
	}
	l.peers = append(peers, l.self)
	return l, nil
}

func (l *Candidate) Leader() (who string, has bool) {
	const key = "leader"
	l.nodeLock.RLock()
	defer l.nodeLock.RUnlock()
	return l.hashRing.GetNode(key)
}

func (l *Candidate) AddNodes(node ...string) {
	l.nodeLock.Lock()
	defer l.nodeLock.Unlock()
	for _, peer := range node {
		l.hashRing = l.hashRing.AddNode(peer)
	}
}

func (l *Candidate) RemoveNodes(node ...string) {
	l.nodeLock.Lock()
	defer l.nodeLock.Unlock()
	for _, peer := range node {
		l.hashRing = l.hashRing.RemoveNode(peer)
	}
}

func (l *Candidate) Run(ctx context.Context, options ...Option) error {
	g, _ := NewGossip(l.peers, options...)
	l.gossip = g
	watch, err := l.gossip.Watch(ctx)
	if err != nil {
		return err
	}
	l.watch = watch
	go l.loop(ctx)

	var (
		oldCancel  context.CancelFunc
		newCancel  context.CancelFunc
		lctx       context.Context
		roleSwitch bool
	)
	lctx, newCancel = context.WithCancel(ctx)
	roleSwitch = true

	for {
		if roleSwitch {
			oldCancel = newCancel
			lctx, newCancel = context.WithCancel(ctx)
			roleSwitch = false
		}
		select {
		case <-ctx.Done():
			oldCancel()
			newCancel()
			return nil
		case <-l.leaderMayChange:
			who, _ := l.Leader()
			if who == l.self && l.current != Leader && l.leaderAction != nil {
				oldCancel()
				roleSwitch = true
				l.current = Leader

				l.broadcastEvent = nil
				l.queryEvent = nil
				go l.leaderAction(lctx,
					func(b BroadcastEvent) error {
						return l.gossip.serfClient.UserEvent(b.Name, b.Payload, true)
					},
					func(name string, payload []byte) (<-chan serf.NodeResponse, error) {
						res, err := l.gossip.serfClient.Query(name, payload, nil)
						if err != nil {
							return nil, err
						}
						return res.ResponseCh(), nil
					})
			}
			if who != l.self && l.current != Follower && l.followerAction != nil {
				oldCancel()
				roleSwitch = true
				l.current = Follower

				l.broadcastEvent = make(chan BroadcastEvent)
				l.queryEvent = make(chan QueryEvent)
				go l.followerAction(lctx, l.broadcastEvent, l.queryEvent)
			}
		}
	}
}

func (l *Candidate) loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-l.watch:
			switch ev := event.(type) {
			case MemberJoinEvent:
				l.AddNodes(ev.IP...)
				l.leaderMayChange <- struct{}{}
			case MemberRemoveEvent:
				l.RemoveNodes(ev.IP...)
				l.leaderMayChange <- struct{}{}
			case BroadcastEvent:
				select {
				case l.broadcastEvent <- ev:
				default:
				}
			case QueryEvent:
				select {
				case l.queryEvent <- ev:
				default:
				}
			default:
			}
		}
	}

}
