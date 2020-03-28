// +build !js

package webrtc

import "testing"

func TestGenerateDataChannelID(t *testing.T) {
	sctpTransportWithChannels := func(ids []uint16) *SCTPTransport {
		ret := &SCTPTransport{dataChannels: []*DataChannel{}}

		for i := range ids {
			ret.dataChannels = append(ret.dataChannels, &DataChannel{})
			ret.dataChannels[len(ret.dataChannels)-1].id.Store(&ids[i])
		}

		return ret
	}

	testCases := []struct {
		role   DTLSRole
		s      *SCTPTransport
		result uint16
	}{
		{DTLSRoleClient, sctpTransportWithChannels([]uint16{}), 0},
		{DTLSRoleClient, sctpTransportWithChannels([]uint16{1}), 0},
		{DTLSRoleClient, sctpTransportWithChannels([]uint16{0}), 2},
		{DTLSRoleClient, sctpTransportWithChannels([]uint16{0, 2}), 4},
		{DTLSRoleClient, sctpTransportWithChannels([]uint16{0, 4}), 2},
		{DTLSRoleServer, sctpTransportWithChannels([]uint16{}), 1},
		{DTLSRoleServer, sctpTransportWithChannels([]uint16{0}), 1},
		{DTLSRoleServer, sctpTransportWithChannels([]uint16{1}), 3},
		{DTLSRoleServer, sctpTransportWithChannels([]uint16{1, 3}), 5},
		{DTLSRoleServer, sctpTransportWithChannels([]uint16{1, 5}), 3},
	}
	for _, testCase := range testCases {
		id, err := testCase.s.generateDataChannelID(testCase.role)
		if err != nil {
			t.Errorf("failed to generate id: %v", err)
			return
		}
		if id != testCase.result {
			t.Errorf("Wrong id: %d expected %d", id, testCase.result)
		}
	}
}
