// +build !js

package webrtc

import (
	"fmt"
	"sync/atomic"

	"github.com/pion/rtp"
)

// RTPTransceiver represents a combination of an RTPSender and an RTPReceiver that share a common mid.
type RTPTransceiver struct {
	mid       atomic.Value // string; set only once
	sender    atomic.Value // *RTPSender
	receiver  atomic.Value // *RTPReceiver
	direction atomic.Value // RTPTransceiverDirection
	stopped   atomicBool
	kind      RTPCodecType
}

func newRTPTransceiver(
	receiver *RTPReceiver,
	sender *RTPSender,
	direction RTPTransceiverDirection,
	kind RTPCodecType,
) *RTPTransceiver {
	t := &RTPTransceiver{kind: kind}
	t.setReceiver(receiver)
	t.setSender(sender)
	t.setDirection(direction)
	return t
}

// Sender returns the RTPTransceiver's RTPSender if it has one
func (t *RTPTransceiver) Sender() *RTPSender {
	if v := t.sender.Load(); v != nil {
		return v.(*RTPSender)
	}

	return nil
}

// SetSender sets the RTPSender and Track to current transceiver
func (t *RTPTransceiver) SetSender(s *RTPSender, track TrackLocal) error {
	t.setSender(s)
	return t.setSendingTrack(track)
}

func (t *RTPTransceiver) setSender(s *RTPSender) {
	t.sender.Store(s)
}

// Receiver returns the RTPTransceiver's RTPReceiver if it has one
func (t *RTPTransceiver) Receiver() *RTPReceiver {
	if v := t.receiver.Load(); v != nil {
		return v.(*RTPReceiver)
	}

	return nil
}

// setMid sets the RTPTransceiver's mid. If it was already set, will return an error.
func (t *RTPTransceiver) setMid(mid string) error {
	if currentMid := t.Mid(); currentMid != "" {
		return fmt.Errorf("%w: %s to %s", errRTPTransceiverCannotChangeMid, currentMid, mid)
	}
	t.mid.Store(mid)
	return nil
}

// Mid gets the Transceiver's mid value. When not already set, this value will be set in CreateOffer or CreateAnswer.
func (t *RTPTransceiver) Mid() string {
	if v := t.mid.Load(); v != nil {
		return v.(string)
	}
	return ""
}

// Kind returns RTPTransceiver's kind.
func (t *RTPTransceiver) Kind() RTPCodecType {
	return t.kind
}

// Direction returns the RTPTransceiver's current direction
func (t *RTPTransceiver) Direction() RTPTransceiverDirection {
	return t.direction.Load().(RTPTransceiverDirection)
}

// Stop irreversibly stops the RTPTransceiver
func (t *RTPTransceiver) Stop() error {
	if t.stopped.compareAndSwap(false, true) {
		if sender := t.Sender(); sender != nil {
			if err := sender.Stop(); err != nil {
				return err
			}
		}
		if receiver := t.Receiver(); receiver != nil {
			if err := receiver.Stop(); err != nil {
				return err
			}
		}

		t.setDirection(RTPTransceiverDirectionInactive)
	}
	return nil
}

// Stopped indicates whether or not RTPTransceiver has been stopped
func (t *RTPTransceiver) Stopped() bool { return t.stopped.get() }

func (t *RTPTransceiver) setReceiver(r *RTPReceiver) {
	t.receiver.Store(r)
}

func (t *RTPTransceiver) setDirection(d RTPTransceiverDirection) {
	t.direction.Store(d)
}

func (t *RTPTransceiver) setSendingTrack(track TrackLocal) error {
	if err := t.Sender().ReplaceTrack(track); err != nil {
		return err
	}
	if track == nil {
		t.setSender(nil)
	}

	switch {
	case track != nil && t.Direction() == RTPTransceiverDirectionRecvonly:
		t.setDirection(RTPTransceiverDirectionSendrecv)
	case track != nil && t.Direction() == RTPTransceiverDirectionInactive:
		t.setDirection(RTPTransceiverDirectionSendonly)
	case track == nil && t.Direction() == RTPTransceiverDirectionSendrecv:
		t.setDirection(RTPTransceiverDirectionRecvonly)
	case track != nil && t.Direction() == RTPTransceiverDirectionSendonly:
		// Handle the case where a sendonly transceiver was added by a negotiation
		// initiated by remote peer. For example a remote peer added a transceiver
		// with direction recvonly.
	case track != nil && t.Direction() == RTPTransceiverDirectionSendrecv:
		// Similar to above, but for sendrecv transceiver.
	case track == nil && t.Direction() == RTPTransceiverDirectionSendonly:
		t.setDirection(RTPTransceiverDirectionInactive)
	default:
		return errRTPTransceiverSetSendingInvalidState
	}
	return nil
}

func findByMid(mid string, localTransceivers []*RTPTransceiver) (*RTPTransceiver, []*RTPTransceiver) {
	for i, t := range localTransceivers {
		if t.Mid() == mid {
			return t, append(localTransceivers[:i], localTransceivers[i+1:]...)
		}
	}

	return nil, localTransceivers
}

// Given a direction+type pluck a transceiver from the passed list
// if no entry satisfies the requested type+direction return a inactive Transceiver
func satisfyTypeAndDirection(remoteKind RTPCodecType, remoteDirection RTPTransceiverDirection, localTransceivers []*RTPTransceiver) (*RTPTransceiver, []*RTPTransceiver) {
	// Get direction order from most preferred to least
	getPreferredDirections := func() []RTPTransceiverDirection {
		switch remoteDirection {
		case RTPTransceiverDirectionSendrecv:
			return []RTPTransceiverDirection{RTPTransceiverDirectionRecvonly, RTPTransceiverDirectionSendrecv}
		case RTPTransceiverDirectionSendonly:
			return []RTPTransceiverDirection{RTPTransceiverDirectionRecvonly}
		case RTPTransceiverDirectionRecvonly:
			return []RTPTransceiverDirection{RTPTransceiverDirectionSendonly, RTPTransceiverDirectionSendrecv}
		default:
			return []RTPTransceiverDirection{}
		}
	}

	for _, possibleDirection := range getPreferredDirections() {
		for i := range localTransceivers {
			t := localTransceivers[i]
			if t.Mid() == "" && t.kind == remoteKind && possibleDirection == t.Direction() {
				return t, append(localTransceivers[:i], localTransceivers[i+1:]...)
			}
		}
	}

	return nil, localTransceivers
}

// handleUnknownRTPPacket consumes a single RTP Packet and returns information that is helpful
// for demuxing and handling an unknown SSRC (usually for Simulcast)
func handleUnknownRTPPacket(buf []byte, midExtensionID, streamIDExtensionID uint8) (mid, rid string, payloadType PayloadType, err error) {
	rp := &rtp.Packet{}
	if err = rp.Unmarshal(buf); err != nil {
		return
	}

	if !rp.Header.Extension {
		return
	}

	payloadType = PayloadType(rp.PayloadType)
	if payload := rp.GetExtension(midExtensionID); payload != nil {
		mid = string(payload)
	}

	if payload := rp.GetExtension(streamIDExtensionID); payload != nil {
		rid = string(payload)
	}

	return
}
