package samplebuilder

import (
	"fmt"
	"testing"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3/pkg/media"
	"github.com/stretchr/testify/assert"
)

type sampleBuilderTest struct {
	message          string
	packets          []*rtp.Packet
	headBytes        []byte
	tailChecker      func(*rtp.Packet) bool
	samples          []*media.Sample
	maxLate          uint16
	maxLateTimestamp uint32
}

type fakeDepacketizer struct{}

func (f *fakeDepacketizer) Unmarshal(r []byte) ([]byte, error) {
	return r, nil
}

func (f *fakeDepacketizer) IsDetectedFinalPacketInSequence(rtpPacketMarketBit bool) bool {
	return rtpPacketMarketBit
}

type fakePartitionHeadChecker struct {
	headBytes []byte
}

func (f *fakePartitionHeadChecker) IsPartitionHead(payload []byte) bool {
	for _, b := range f.headBytes {
		if payload[0] == b {
			return true
		}
	}
	return false
}

func fakeTailChecker(p *rtp.Packet) bool {
	return p.Marker
}

func TestSampleBuilder(t *testing.T) {
	testData := []sampleBuilderTest{
		{
			message: "SampleBuilder shouldn't emit anything if only one RTP packet has been pushed",
			packets: []*rtp.Packet{
				{Header: rtp.Header{SequenceNumber: 5000, Timestamp: 5}, Payload: []byte{0x01}},
			},
			samples:          []*media.Sample{},
			maxLate:          50,
			maxLateTimestamp: 0,
			tailChecker:      fakeTailChecker,
		},
		{
			message: "SampleBuilder shouldn't emit anything if only one RTP packet has been pushed even if the market bit is set",
			packets: []*rtp.Packet{
				{Header: rtp.Header{SequenceNumber: 5000, Timestamp: 5, Marker: true}, Payload: []byte{0x01}},
			},
			samples:          []*media.Sample{},
			maxLate:          50,
			maxLateTimestamp: 0,
			tailChecker:      fakeTailChecker,
		},
		{
			message: "SampleBuilder should emit two packets, we had three packets with unique timestamps",
			packets: []*rtp.Packet{
				{Header: rtp.Header{SequenceNumber: 5000, Timestamp: 5}, Payload: []byte{0x01}},
				{Header: rtp.Header{SequenceNumber: 5001, Timestamp: 6}, Payload: []byte{0x02}},
				{Header: rtp.Header{SequenceNumber: 5002, Timestamp: 7}, Payload: []byte{0x03}},
			},
			samples: []*media.Sample{
				{Data: []byte{0x01}, Duration: time.Second, PacketTimestamp: 5},
				{Data: []byte{0x02}, Duration: time.Second, PacketTimestamp: 6},
			},
			maxLate:          50,
			maxLateTimestamp: 0,
			tailChecker:      fakeTailChecker,
		},
		{
			message: "SampleBuilder should emit one packet, we had a packet end of sequence marker and run out of space",
			packets: []*rtp.Packet{
				{Header: rtp.Header{SequenceNumber: 5000, Timestamp: 5, Marker: true}, Payload: []byte{0x01}},
				{Header: rtp.Header{SequenceNumber: 5002, Timestamp: 7}, Payload: []byte{0x02}},
				{Header: rtp.Header{SequenceNumber: 5004, Timestamp: 9}, Payload: []byte{0x03}},
				{Header: rtp.Header{SequenceNumber: 5006, Timestamp: 11}, Payload: []byte{0x04}},
				{Header: rtp.Header{SequenceNumber: 5008, Timestamp: 13}, Payload: []byte{0x05}},
				{Header: rtp.Header{SequenceNumber: 5010, Timestamp: 15}, Payload: []byte{0x06}},
				{Header: rtp.Header{SequenceNumber: 5012, Timestamp: 17}, Payload: []byte{0x07}},
			},
			samples: []*media.Sample{
				{Data: []byte{0x01}, Duration: time.Second * 2, PacketTimestamp: 5},
			},
			maxLate:          5,
			maxLateTimestamp: 0,
			tailChecker:      fakeTailChecker,
		},
		{
			message: "SampleBuilder shouldn't emit any packet, we do not have a valid end of sequence and run out of space",
			packets: []*rtp.Packet{
				{Header: rtp.Header{SequenceNumber: 5000, Timestamp: 5}, Payload: []byte{0x01}},
				{Header: rtp.Header{SequenceNumber: 5002, Timestamp: 7}, Payload: []byte{0x02}},
				{Header: rtp.Header{SequenceNumber: 5004, Timestamp: 9}, Payload: []byte{0x03}},
				{Header: rtp.Header{SequenceNumber: 5006, Timestamp: 11}, Payload: []byte{0x04}},
				{Header: rtp.Header{SequenceNumber: 5008, Timestamp: 13}, Payload: []byte{0x05}},
				{Header: rtp.Header{SequenceNumber: 5010, Timestamp: 15}, Payload: []byte{0x06}},
				{Header: rtp.Header{SequenceNumber: 5012, Timestamp: 17}, Payload: []byte{0x07}},
			},
			samples:          []*media.Sample{},
			maxLate:          5,
			maxLateTimestamp: 0,
			tailChecker:      fakeTailChecker,
		},
		{
			message: "SampleBuilder should emit one packet, we had a packet end of sequence marker and run out of space",
			packets: []*rtp.Packet{
				{Header: rtp.Header{SequenceNumber: 5000, Timestamp: 5, Marker: true}, Payload: []byte{0x01}},
				{Header: rtp.Header{SequenceNumber: 5002, Timestamp: 7, Marker: true}, Payload: []byte{0x02}},
				{Header: rtp.Header{SequenceNumber: 5004, Timestamp: 9}, Payload: []byte{0x03}},
				{Header: rtp.Header{SequenceNumber: 5006, Timestamp: 11}, Payload: []byte{0x04}},
				{Header: rtp.Header{SequenceNumber: 5008, Timestamp: 13}, Payload: []byte{0x05}},
				{Header: rtp.Header{SequenceNumber: 5010, Timestamp: 15}, Payload: []byte{0x06}},
				{Header: rtp.Header{SequenceNumber: 5012, Timestamp: 17}, Payload: []byte{0x07}},
			},
			samples: []*media.Sample{
				{Data: []byte{0x01}, Duration: time.Second * 2, PacketTimestamp: 5},
				{Data: []byte{0x02}, Duration: time.Second * 2, PacketTimestamp: 7, PrevDroppedPackets: 1},
			},
			maxLate:          5,
			maxLateTimestamp: 0,
			tailChecker:      fakeTailChecker,
		},
		{
			message: "SampleBuilder should emit one packet, we had two packets but two with duplicate timestamps",
			packets: []*rtp.Packet{
				{Header: rtp.Header{SequenceNumber: 5000, Timestamp: 5}, Payload: []byte{0x01}},
				{Header: rtp.Header{SequenceNumber: 5001, Timestamp: 6}, Payload: []byte{0x02}},
				{Header: rtp.Header{SequenceNumber: 5002, Timestamp: 6}, Payload: []byte{0x03}},
				{Header: rtp.Header{SequenceNumber: 5003, Timestamp: 7}, Payload: []byte{0x04}},
			},
			samples: []*media.Sample{
				{Data: []byte{0x01}, Duration: time.Second, PacketTimestamp: 5},
				{Data: []byte{0x02, 0x03}, Duration: time.Second, PacketTimestamp: 6},
			},
			maxLate:          50,
			maxLateTimestamp: 0,
			tailChecker:      fakeTailChecker,
		},
		{
			message: "SampleBuilder shouldn't emit a packet because we have a gap before a valid one",
			packets: []*rtp.Packet{
				{Header: rtp.Header{SequenceNumber: 5000, Timestamp: 5}, Payload: []byte{0x01}},
				{Header: rtp.Header{SequenceNumber: 5007, Timestamp: 6}, Payload: []byte{0x02}},
				{Header: rtp.Header{SequenceNumber: 5008, Timestamp: 7}, Payload: []byte{0x03}},
			},
			samples:          []*media.Sample{},
			maxLate:          50,
			maxLateTimestamp: 0,
			tailChecker:      fakeTailChecker,
		},
		{
			message: "SampleBuilder shouldn't emit a packet after a gap as there are gaps and have not reached maxLate yet",
			packets: []*rtp.Packet{
				{Header: rtp.Header{SequenceNumber: 5000, Timestamp: 5}, Payload: []byte{0x01}},
				{Header: rtp.Header{SequenceNumber: 5007, Timestamp: 6}, Payload: []byte{0x02}},
				{Header: rtp.Header{SequenceNumber: 5008, Timestamp: 7}, Payload: []byte{0x03}},
			},
			headBytes:        []byte{0x02},
			samples:          []*media.Sample{},
			maxLate:          50,
			maxLateTimestamp: 0,
			tailChecker:      fakeTailChecker,
		},
		{
			message: "SampleBuilder shouldn't emit a packet after a gap if PartitionHeadChecker doesn't assume it head",
			packets: []*rtp.Packet{
				{Header: rtp.Header{SequenceNumber: 5000, Timestamp: 5}, Payload: []byte{0x01}},
				{Header: rtp.Header{SequenceNumber: 5007, Timestamp: 6}, Payload: []byte{0x02}},
				{Header: rtp.Header{SequenceNumber: 5008, Timestamp: 7}, Payload: []byte{0x03}},
			},
			headBytes:        []byte{},
			samples:          []*media.Sample{},
			maxLate:          50,
			maxLateTimestamp: 0,
			tailChecker:      fakeTailChecker,
		},
		{
			message: "SampleBuilder should emit multiple valid packets",
			packets: []*rtp.Packet{
				{Header: rtp.Header{SequenceNumber: 5000, Timestamp: 1}, Payload: []byte{0x01}},
				{Header: rtp.Header{SequenceNumber: 5001, Timestamp: 2}, Payload: []byte{0x02}},
				{Header: rtp.Header{SequenceNumber: 5002, Timestamp: 3}, Payload: []byte{0x03}},
				{Header: rtp.Header{SequenceNumber: 5003, Timestamp: 4}, Payload: []byte{0x04}},
				{Header: rtp.Header{SequenceNumber: 5004, Timestamp: 5}, Payload: []byte{0x05}},
				{Header: rtp.Header{SequenceNumber: 5005, Timestamp: 6}, Payload: []byte{0x06}},
			},
			samples: []*media.Sample{
				{Data: []byte{0x01}, Duration: time.Second, PacketTimestamp: 1},
				{Data: []byte{0x02}, Duration: time.Second, PacketTimestamp: 2},
				{Data: []byte{0x03}, Duration: time.Second, PacketTimestamp: 3},
				{Data: []byte{0x04}, Duration: time.Second, PacketTimestamp: 4},
				{Data: []byte{0x05}, Duration: time.Second, PacketTimestamp: 5},
			},
			maxLate:          50,
			maxLateTimestamp: 0,
			tailChecker:      fakeTailChecker,
		},
		{
			message: "SampleBuilder should skip time stamps too old",
			packets: []*rtp.Packet{
				{Header: rtp.Header{SequenceNumber: 5000, Timestamp: 1}, Payload: []byte{0x01}},
				{Header: rtp.Header{SequenceNumber: 5001, Timestamp: 2}, Payload: []byte{0x02}},
				{Header: rtp.Header{SequenceNumber: 5002, Timestamp: 3}, Payload: []byte{0x03}},
				{Header: rtp.Header{SequenceNumber: 5013, Timestamp: 4000}, Payload: []byte{0x04}},
				{Header: rtp.Header{SequenceNumber: 5014, Timestamp: 4000}, Payload: []byte{0x05}},
				{Header: rtp.Header{SequenceNumber: 5015, Timestamp: 4002}, Payload: []byte{0x06}},
				{Header: rtp.Header{SequenceNumber: 5016, Timestamp: 7000}, Payload: []byte{0x04}},
				{Header: rtp.Header{SequenceNumber: 5017, Timestamp: 7001}, Payload: []byte{0x05}},
			},
			samples: []*media.Sample{
				{Data: []byte{0x04, 0x05}, Duration: time.Second * time.Duration(2), PacketTimestamp: 4000, PrevDroppedPackets: 13},
			},
			headBytes:        []byte{0x04},
			maxLate:          50,
			maxLateTimestamp: 2000,
			tailChecker:      fakeTailChecker,
		},
	}

	t.Run("Pop", func(t *testing.T) {
		assert := assert.New(t)

		for _, t := range testData {
			var opts []Option
			if len(t.headBytes) > 0 {
				opts = append(opts, WithPartitionHeadChecker(
					&fakePartitionHeadChecker{headBytes: t.headBytes},
				))
			}
			if t.tailChecker != nil {
				opts = append(opts, WithPartitionTailChecker(
					t.tailChecker,
				))
			}
			if t.maxLateTimestamp != 0 {
				opts = append(opts, WithMaxTimeDelay(
					time.Millisecond*time.Duration(int64(t.maxLateTimestamp)),
				))
			}

			s := New(t.maxLate, &fakeDepacketizer{}, 1, opts...)
			samples := []*media.Sample{}

			for _, p := range t.packets {
				s.Push(p)
			}
			for sample := s.Pop(); sample != nil; sample = s.Pop() {
				samples = append(samples, sample)
			}
			assert.Equal(t.samples, samples, t.message)
		}
	})
}

// SampleBuilder should respect maxLate if we popped successfully but then have a gap larger then maxLate
func TestSampleBuilderMaxLate(t *testing.T) {
	assert := assert.New(t)
	s := New(50, &fakeDepacketizer{}, 1)

	s.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 0, Timestamp: 1}, Payload: []byte{0x01}})
	s.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 1, Timestamp: 2}, Payload: []byte{0x01}})
	s.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 2, Timestamp: 3}, Payload: []byte{0x01}})
	assert.Equal(&media.Sample{Data: []byte{0x01}, Duration: time.Second, PacketTimestamp: 1}, s.Pop(), "Failed to build samples before gap")

	s.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5000, Timestamp: 500}, Payload: []byte{0x02}})
	s.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5001, Timestamp: 501}, Payload: []byte{0x02}})
	s.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 5002, Timestamp: 502}, Payload: []byte{0x02}})

	assert.Equal(&media.Sample{Data: []byte{0x01}, Duration: time.Second, PacketTimestamp: 2}, s.Pop(), "Failed to build samples after large gap")
	assert.Equal((*media.Sample)(nil), s.Pop(), "Failed to build samples after large gap")

	s.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 6000, Timestamp: 600}, Payload: []byte{0x03}})
	assert.Equal(&media.Sample{Data: []byte{0x02}, Duration: time.Second, PacketTimestamp: 500, PrevDroppedPackets: 4998}, s.Pop(), "Failed to build samples after large gap")
	assert.Equal(&media.Sample{Data: []byte{0x02}, Duration: time.Second, PacketTimestamp: 501}, s.Pop(), "Failed to build samples after large gap")
}

func TestSeqnumDistance(t *testing.T) {
	testData := []struct {
		x uint16
		y uint16
		d uint16
	}{
		{0x0001, 0x0003, 0x0002},
		{0x0003, 0x0001, 0x0002},
		{0xFFF3, 0xFFF1, 0x0002},
		{0xFFF1, 0xFFF3, 0x0002},
		{0xFFFF, 0x0001, 0x0002},
		{0x0001, 0xFFFF, 0x0002},
	}

	for _, data := range testData {
		if ret := seqnumDistance(data.x, data.y); ret != data.d {
			t.Errorf("seqnumDistance(%d, %d) returned %d which must be %d",
				data.x, data.y, ret, data.d)
		}
	}
}

func TestSampleBuilderCleanReference(t *testing.T) {
	for _, seqStart := range []uint16{
		0,
		0xFFF8, // check upper boundary
		0xFFFE, // check upper boundary
	} {
		seqStart := seqStart
		t.Run(fmt.Sprintf("From%d", seqStart), func(t *testing.T) {
			s := New(10, &fakeDepacketizer{}, 1)

			s.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 0 + seqStart, Timestamp: 0}, Payload: []byte{0x01}})
			s.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 1 + seqStart, Timestamp: 0}, Payload: []byte{0x02}})
			s.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 2 + seqStart, Timestamp: 0}, Payload: []byte{0x03}})
			pkt4 := &rtp.Packet{Header: rtp.Header{SequenceNumber: 14 + seqStart, Timestamp: 120}, Payload: []byte{0x04}}
			s.Push(pkt4)
			pkt5 := &rtp.Packet{Header: rtp.Header{SequenceNumber: 12 + seqStart, Timestamp: 120}, Payload: []byte{0x05}}
			s.Push(pkt5)

			for i := 0; i < 3; i++ {
				if s.buffer[(i+int(seqStart))%0x10000] != nil {
					t.Errorf("Old packet (%d) is not unreferenced (maxLate: 10, pushed: 12)", i)
				}
			}
			if s.buffer[(14+int(seqStart))%0x10000] != pkt4 {
				t.Error("New packet must be referenced after jump")
			}
			if s.buffer[(12+int(seqStart))%0x10000] != pkt5 {
				t.Error("New packet must be referenced after jump")
			}
		})
	}
}

func TestSampleBuilderPushMaxZero(t *testing.T) {
	// Test packets released via 'maxLate' of zero.
	pkts := []rtp.Packet{
		{Header: rtp.Header{SequenceNumber: 0, Timestamp: 0, Marker: true}, Payload: []byte{0x01}},
	}
	s := New(0, &fakeDepacketizer{}, 1,
		WithPartitionHeadChecker(
			&fakePartitionHeadChecker{headBytes: []byte{0x01}},
		),
		WithPartitionTailChecker(
			fakeTailChecker,
		),
	)
	s.Push(&pkts[0])
	if sample := s.Pop(); sample == nil {
		t.Error("Should expect a popped sample")
	}
}

func TestSampleBuilderWithPacketReleaseHandler(t *testing.T) {
	var released []*rtp.Packet
	fakePacketReleaseHandler := func(p *rtp.Packet) {
		released = append(released, p)
	}

	// Test packets released via 'maxLate'.
	pkts := []rtp.Packet{
		{Header: rtp.Header{SequenceNumber: 0, Timestamp: 0}, Payload: []byte{0x01}},
		{Header: rtp.Header{SequenceNumber: 11, Timestamp: 120}, Payload: []byte{0x02}},
		{Header: rtp.Header{SequenceNumber: 12, Timestamp: 121}, Payload: []byte{0x03}},
		{Header: rtp.Header{SequenceNumber: 13, Timestamp: 122}, Payload: []byte{0x04}},
		{Header: rtp.Header{SequenceNumber: 21, Timestamp: 200}, Payload: []byte{0x05}},
	}
	s := New(10, &fakeDepacketizer{}, 1, WithPacketReleaseHandler(fakePacketReleaseHandler))
	s.Push(&pkts[0])
	s.Push(&pkts[1])
	if len(released) == 0 {
		t.Errorf("Old packet is not released")
	}
	if len(released) > 0 && released[0].SequenceNumber != pkts[0].SequenceNumber {
		t.Errorf("Unexpected packet released by maxLate")
	}
	// Test packets released after samples built.
	s.Push(&pkts[2])
	s.Push(&pkts[3])
	s.Push(&pkts[4])
	if s.Pop() == nil {
		t.Errorf("Should have some sample here.")
	}
	if len(released) < 3 {
		t.Errorf("packet built with sample is not released")
	}
	if len(released) >= 2 && released[2].SequenceNumber != pkts[2].SequenceNumber {
		t.Errorf("Unexpected packet released by samples built")
	}
}

func TestPopWithTimestamp(t *testing.T) {
	t.Run("Crash on nil", func(t *testing.T) {
		s := New(0, &fakeDepacketizer{}, 1)
		sample, timestamp := s.PopWithTimestamp()
		assert.Nil(t, sample)
		assert.Equal(t, uint32(0), timestamp)
	})
}

type truePartitionHeadChecker struct{}

func (f *truePartitionHeadChecker) IsPartitionHead(payload []byte) bool {
	return true
}

func TestSampleBuilderData(t *testing.T) {
	s := New(10, &fakeDepacketizer{}, 1,
		WithPartitionHeadChecker(&truePartitionHeadChecker{}),
	)
	j := 0
	for i := 0; i < 0x20000; i++ {
		p := rtp.Packet{
			Header: rtp.Header{
				SequenceNumber: uint16(i),
				Timestamp:      uint32(i + 42),
			},
			Payload: []byte{byte(i)},
		}
		s.Push(&p)
		for {
			sample, ts := s.PopWithTimestamp()
			if sample == nil {
				break
			}
			assert.Equal(t, ts, uint32(j+42), "timestamp")
			assert.Equal(t, len(sample.Data), 1, "data length")
			assert.Equal(t, byte(j), sample.Data[0], "data")
			j++
		}
	}
	// only the last packet should be dropped
	assert.Equal(t, j, 0x1FFFF)
}

func BenchmarkSampleBuilderSequential(b *testing.B) {
	s := New(100, &fakeDepacketizer{}, 1)
	b.ResetTimer()
	j := 0
	for i := 0; i < b.N; i++ {
		p := rtp.Packet{
			Header: rtp.Header{
				SequenceNumber: uint16(i),
				Timestamp:      uint32(i + 42),
			},
			Payload: make([]byte, 50),
		}
		s.Push(&p)
		for {
			s := s.Pop()
			if s == nil {
				break
			}
			j++
		}
	}
	if b.N > 200 && j < b.N-100 {
		b.Errorf("Got %v (N=%v)", j, b.N)
	}
}

func BenchmarkSampleBuilderLoss(b *testing.B) {
	s := New(100, &fakeDepacketizer{}, 1)
	b.ResetTimer()
	j := 0
	for i := 0; i < b.N; i++ {
		if i%13 == 0 {
			continue
		}
		p := rtp.Packet{
			Header: rtp.Header{
				SequenceNumber: uint16(i),
				Timestamp:      uint32(i + 42),
			},
			Payload: make([]byte, 50),
		}
		s.Push(&p)
		for {
			s := s.Pop()
			if s == nil {
				break
			}
			j++
		}
	}
	if b.N > 200 && j < b.N/2-100 {
		b.Errorf("Got %v (N=%v)", j, b.N)
	}
}

func BenchmarkSampleBuilderReordered(b *testing.B) {
	s := New(100, &fakeDepacketizer{}, 1)
	b.ResetTimer()
	j := 0
	for i := 0; i < b.N; i++ {
		p := rtp.Packet{
			Header: rtp.Header{
				SequenceNumber: uint16(i ^ 3),
				Timestamp:      uint32((i ^ 3) + 42),
			},
			Payload: make([]byte, 50),
		}
		s.Push(&p)
		for {
			s := s.Pop()
			if s == nil {
				break
			}
			j++
		}
	}
	if b.N > 2 && j < b.N-5 && j > b.N {
		b.Errorf("Got %v (N=%v)", j, b.N)
	}
}

func BenchmarkSampleBuilderFragmented(b *testing.B) {
	s := New(100, &fakeDepacketizer{}, 1)
	b.ResetTimer()
	j := 0
	for i := 0; i < b.N; i++ {
		p := rtp.Packet{
			Header: rtp.Header{
				SequenceNumber: uint16(i),
				Timestamp:      uint32(i/2 + 42),
			},
			Payload: make([]byte, 50),
		}
		s.Push(&p)
		for {
			s := s.Pop()
			if s == nil {
				break
			}
			j++
		}
	}
	if b.N > 200 && j < b.N/2-100 {
		b.Errorf("Got %v (N=%v)", j, b.N)
	}
}

func BenchmarkSampleBuilderFragmentedLoss(b *testing.B) {
	s := New(100, &fakeDepacketizer{}, 1)
	b.ResetTimer()
	j := 0
	for i := 0; i < b.N; i++ {
		if i%13 == 0 {
			continue
		}
		p := rtp.Packet{
			Header: rtp.Header{
				SequenceNumber: uint16(i),
				Timestamp:      uint32(i/2 + 42),
			},
			Payload: make([]byte, 50),
		}
		s.Push(&p)
		for {
			s := s.Pop()
			if s == nil {
				break
			}
			j++
		}
	}
	if b.N > 200 && j < b.N/3-100 {
		b.Errorf("Got %v (N=%v)", j, b.N)
	}
}
