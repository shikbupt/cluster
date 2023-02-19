package cluster

import (
	"context"

	"github.com/hashicorp/serf/serf"
)

type GossipMembership struct {
	peersAddress   []string
	Config         *serf.Config
	serfClient     *serf.Serf
	broadCastEvent serf.UserEvent
	eventCh        chan serf.Event
	latestLTtime   serf.LamportTime
}

func NewGossip(peers []string) (*GossipMembership, error) {
	g := &GossipMembership{
		peersAddress: peers,
		Config:       serf.DefaultConfig(),
	}
	g.eventCh = make(chan serf.Event, 16)

	g.Config.EventCh = g.eventCh

	return g, nil
}

func (g *GossipMembership) Watch(ctx context.Context) (<-chan Event, error) {
	serfClient, err := serf.Create(g.Config)
	if err != nil {
		return nil, err
	}
	g.serfClient = serfClient

	_, err = serfClient.Join(g.peersAddress, true)
	if err != nil {
		return nil, err
	}
	eventCh := make(chan Event)

	go g.loop(ctx, eventCh)
	return eventCh, nil
}

func (g *GossipMembership) loop(ctx context.Context, eventCh chan Event) {
	for {
		select {
		case <-ctx.Done():
			close(eventCh)
			return
		case ev := <-g.eventCh:
			if memberEvent, ok := ev.(serf.MemberEvent); ok {
				updatedPeers := make([]string, 0, len(memberEvent.Members))
				for _, member := range memberEvent.Members {
					updatedPeers = append(updatedPeers, member.Addr.String())
				}
				switch memberEvent.EventType() {
				case serf.EventMemberJoin:
					eventCh <- MemberJoinEvent{
						memberChangeEvent: memberChangeEvent{updatedPeers},
					}
				case serf.EventMemberLeave, serf.EventMemberFailed:
					eventCh <- MemberLeftEvent{
						memberChangeEvent: memberChangeEvent{updatedPeers},
					}
				case serf.EventMemberReap:
					eventCh <- MemberRemoveEvent{
						memberChangeEvent: memberChangeEvent{updatedPeers},
					}
				default:
				}
			}
			if userEvent, ok := ev.(serf.UserEvent); ok {
				if g.latestLTtime > userEvent.LTime {
					continue
				}
				g.latestLTtime = userEvent.LTime
				eventCh <- BroadcastEvent{
					Name:    userEvent.Name,
					Payload: userEvent.Payload,
				}
			}
		default:
		}
	}
}

func (g *GossipMembership) GetActiveMembers() Members {
	members := g.serfClient.Members()
	IP := make([]string, 0, len(members))
	for index := range members {
		if members[index].Status == serf.StatusAlive {
			IP = append(IP, members[index].Addr.String())
		}
	}
	return Members{
		IP: IP,
	}
}
