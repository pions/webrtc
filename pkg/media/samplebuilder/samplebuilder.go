// Package samplebuilder provides functionality to reconstruct media frames from RTP packets.
package samplebuilder

import (
	"math"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3/pkg/media"
)

// SampleBuilder buffers packets until media frames are complete.
type SampleBuilder struct {
	maxLate          uint16 // how many packets to wait until we get a valid Sample
	maxLateTimestamp uint32 // max timestamp between old and new timestamps before dropping packets
	buffer           [math.MaxUint16 + 1]*rtp.Packet
	preparedSamples  [math.MaxUint16 + 1]*media.Sample

	// Interface that allows us to take RTP packets to samples
	depacketizer rtp.Depacketizer

	// sampleRate allows us to compute duration of media.SamplecA
	sampleRate uint32

	// determines whether the packet is the first fragment of the frame
	partitionHeadChecker rtp.PartitionHeadChecker

	// determines whether the packet is the last fragment of a frame
	partitionTailChecker func(*rtp.Packet) bool

	// the handler to be called when the builder is about to remove the
	// reference to some packet.
	packetReleaseHandler func(*rtp.Packet)

	// filled contains the head/tail of the packets inserted into the buffer
	filled sampleSequenceLocation

	// active contains the active head/tail of the timestamp being actively processed
	active sampleSequenceLocation

	// prepared contains the samples that have been processed to date
	prepared sampleSequenceLocation

	// number of packets forced to be dropped
	droppedPackets uint16
}

// New constructs a new SampleBuilder.
// maxLate is how long to wait until we can construct a completed media.Sample.
// maxLate is measured in RTP packet sequence numbers.
// A large maxLate will result in less packet loss but higher latency.
// The depacketizer extracts media samples from RTP packets.
// Several depacketizers are available in package github.com/pion/rtp/codecs.
func New(maxLate uint16, depacketizer rtp.Depacketizer, sampleRate uint32, opts ...Option) *SampleBuilder {
	s := &SampleBuilder{maxLate: maxLate, depacketizer: depacketizer, sampleRate: sampleRate}
	for _, o := range opts {
		o(s)
	}
	return s
}

func (s *SampleBuilder) tooOld(location sampleSequenceLocation) bool {
	if s.maxLateTimestamp == 0 {
		return false
	}

	var foundHead *rtp.Packet
	var foundTail *rtp.Packet

	for i := location.head; i != location.tail; i++ {
		if packet := s.buffer[i]; packet != nil {
			foundHead = packet
			break
		}
	}

	if foundHead == nil {
		return false
	}

	for i := location.tail - 1; i != location.head; i-- {
		if packet := s.buffer[i]; packet != nil {
			foundTail = packet
			break
		}
	}

	if foundTail == nil {
		return false
	}

	return timestampDistance(foundHead.Timestamp, foundTail.Timestamp) > s.maxLateTimestamp
}

// fetchTimestamp returns the timestamp associated with a given sample location
func (s *SampleBuilder) fetchTimestamp(location sampleSequenceLocation) (timestamp uint32, hasData bool) {
	if location.empty() {
		return 0, false
	}
	packet := s.buffer[location.head]
	if packet == nil {
		return 0, false
	}
	return packet.Timestamp, true
}

func (s *SampleBuilder) releasePacket(i uint16) {
	var p *rtp.Packet
	p, s.buffer[i] = s.buffer[i], nil
	if p != nil && s.packetReleaseHandler != nil {
		s.packetReleaseHandler(p)
	}
}

// purgeConsumedBuffers clears all buffers that have already been consumed by
// popping.
func (s *SampleBuilder) purgeConsumedBuffers() {
	s.purgeConsumedLocation(s.active, false)
}

// purgeConsumedLocation clears all buffers that have already been consumed
// during a sample building method.
func (s *SampleBuilder) purgeConsumedLocation(consume sampleSequenceLocation, forceConsume bool) {
	if !s.filled.hasData() {
		return
	}

	switch consume.compare(s.filled.head) {
	case slCompareInside:
		if !forceConsume {
			break
		}
		fallthrough
	case slCompareBefore:
		s.releasePacket(s.filled.head)
		s.filled.head++
	}
}

// purgeBuffers flushes all buffers that are already consumed or those buffers
// that are too late to consume.
func (s *SampleBuilder) purgeBuffers() {
	s.purgeConsumedBuffers()

	for (s.tooOld(s.filled) || (s.filled.count() > s.maxLate)) && s.filled.hasData() {
		if s.active.empty() {
			// refill the active based on the filled packets
			s.active = s.filled
		}

		if s.active.hasData() && (s.active.head == s.filled.head) {
			// attempt to force the active packet to be consumed even though
			// outstanding data may be pending arrival
			if s.buildSample(true) != nil {
				continue
			}

			// could not build the sample so drop it
			s.active.head++
			s.droppedPackets++
		}

		s.releasePacket(s.filled.head)
		s.filled.head++
	}
}

// Push adds an RTP Packet to s's buffer.
//
// Push does not copy the input. If you wish to reuse
// this memory make sure to copy before calling Push
func (s *SampleBuilder) Push(p *rtp.Packet) {
	s.buffer[p.SequenceNumber] = p

	switch s.filled.compare(p.SequenceNumber) {
	case slCompareVoid:
		s.filled.head = p.SequenceNumber
		s.filled.tail = p.SequenceNumber + 1
	case slCompareBefore:
		s.filled.head = p.SequenceNumber
	case slCompareAfter:
		s.filled.tail = p.SequenceNumber + 1
	case slCompareInside:
		break
	}
	s.purgeBuffers()
}

const secondToNanoseconds = 1000000000

// buildSample creates a sample from a valid collection of RTP Packets by
// walking forwards building a sample if everything looks good clear and
// update buffer+values
func (s *SampleBuilder) buildSample(purgingBuffers bool) *media.Sample {
	if s.active.empty() {
		s.active = s.filled
	}

	if s.active.empty() {
		return nil
	}

	if s.filled.compare(s.active.tail) == slCompareInside {
		s.active.tail = s.filled.tail
	}

	var consume sampleSequenceLocation

	for i := s.active.head; s.buffer[i] != nil && s.active.compare(i) != slCompareAfter; i++ {
		if s.partitionTailChecker != nil && s.partitionTailChecker(s.buffer[i]) {
			consume.head = s.active.head
			consume.tail = i + 1
			break
		}
		headTimestamp, hasData := s.fetchTimestamp(s.active)
		if hasData && s.buffer[i].Timestamp != headTimestamp {
			consume.head = s.active.head
			consume.tail = i
			break
		}
	}

	if consume.empty() {
		return nil
	}

	if !purgingBuffers && s.buffer[consume.tail] == nil {
		// wait for the next packet after this set of packets to arrive
		// to ensure at least one post sample timestamp is known
		// (unless we have to release right now)
		return nil
	}

	sampleTimestamp, _ := s.fetchTimestamp(s.active)
	afterTimestamp := sampleTimestamp

	// scan for any packet after the current and use that time stamp as the diff point
	for i := consume.tail; i < s.active.tail; i++ {
		if s.buffer[i] != nil {
			afterTimestamp = s.buffer[i].Timestamp
			break
		}
	}

	// the head set of packets is now fully consumed
	s.active.head = consume.tail

	// prior to decoding all the packets, check if this packet
	// would end being disposed anyway
	if s.partitionHeadChecker != nil {
		if !s.partitionHeadChecker.IsPartitionHead(s.buffer[consume.head].Payload) {
			s.droppedPackets += consume.count()
			s.purgeConsumedLocation(consume, true)
			s.purgeConsumedBuffers()
			return nil
		}
	}

	// merge all the buffers into a sample
	data := []byte{}
	for i := consume.head; i != consume.tail; i++ {
		p, err := s.depacketizer.Unmarshal(s.buffer[i].Payload)
		if err != nil {
			return nil
		}
		data = append(data, p...)
	}
	samples := afterTimestamp - sampleTimestamp

	sample := &media.Sample{
		Data:               data,
		Duration:           time.Duration((float64(samples)/float64(s.sampleRate))*secondToNanoseconds) * time.Nanosecond,
		PacketTimestamp:    sampleTimestamp,
		PrevDroppedPackets: s.droppedPackets,
	}

	s.droppedPackets = 0

	s.preparedSamples[s.prepared.tail] = sample
	s.prepared.tail++

	s.purgeConsumedLocation(consume, true)
	s.purgeConsumedBuffers()

	return sample
}

// Pop compiles pushed RTP packets into media samples and then
// returns the next valid sample (or nil if no sample is compiled).
func (s *SampleBuilder) Pop() *media.Sample {
	_ = s.buildSample(false)
	if s.prepared.empty() {
		return nil
	}
	var result *media.Sample
	result, s.preparedSamples[s.prepared.head] = s.preparedSamples[s.prepared.head], nil
	s.prepared.head++
	return result
}

// PopWithTimestamp compiles pushed RTP packets into media samples and then
// returns the next valid sample with its associated RTP timestamp (or nil, 0 if
// no sample is compiled).
func (s *SampleBuilder) PopWithTimestamp() (*media.Sample, uint32) {
	sample := s.Pop()
	if sample == nil {
		return nil, 0
	}
	return sample, sample.PacketTimestamp
}

// seqnumDistance computes the distance between two sequence numbers
func seqnumDistance(x, y uint16) uint16 {
	diff := int16(x - y)
	if diff < 0 {
		return uint16(-diff)
	}

	return uint16(diff)
}

// timestampDistance computes the distance between two timestamps
func timestampDistance(x, y uint32) uint32 {
	diff := int32(x - y)
	if diff < 0 {
		return uint32(-diff)
	}

	return uint32(diff)
}

// An Option configures a SampleBuilder.
type Option func(o *SampleBuilder)

// WithPartitionHeadChecker assigns a codec-specific PartitionHeadChecker to SampleBuilder.
// Several PartitionHeadCheckers are available in package github.com/pion/rtp/codecs.
func WithPartitionHeadChecker(checker rtp.PartitionHeadChecker) Option {
	return func(o *SampleBuilder) {
		o.partitionHeadChecker = checker
	}
}

// WithPartitionTailChecker assigns a codec-specific PartitionTailChecker.
// For most audio codecs, this should just be
//    func(p *rtp.Packet) bool { return true; }
// For most video codecs, this should be
//    func(p *rtp.Packet) bool { return p.Marker; }
// It is always correct to return false, but it might cause one extra
// packet to be buffered.
func WithPartitionTailChecker(checker func(*rtp.Packet) bool) Option {
	return func(s *SampleBuilder) {
		s.partitionTailChecker = checker
	}
}

// WithPacketReleaseHandler set a callback when the builder is about to release
// some packet.
func WithPacketReleaseHandler(h func(*rtp.Packet)) Option {
	return func(o *SampleBuilder) {
		o.packetReleaseHandler = h
	}
}

// WithMaxTimeDelay ensures that packets that are too old in the buffer get
// purged based on time rather than building up an extraordinarily long delay.
func WithMaxTimeDelay(maxLateDuration time.Duration) Option {
	return func(o *SampleBuilder) {
		totalMillis := maxLateDuration.Milliseconds()
		o.maxLateTimestamp = uint32(int64(o.sampleRate) * totalMillis / 1000)
	}
}
