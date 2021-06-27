// +build !js

package webrtc

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"strings"
	"testing"

	"github.com/pion/sdp/v3"
	"github.com/stretchr/testify/assert"
)

func TestExtractFingerprint(t *testing.T) {
	t.Run("Good Session Fingerprint", func(t *testing.T) {
		s := &sdp.SessionDescription{
			Attributes: []sdp.Attribute{{Key: "fingerprint", Value: "foo bar"}},
		}

		fingerprint, hash, err := extractFingerprint(s)
		assert.NoError(t, err)
		assert.Equal(t, fingerprint, "bar")
		assert.Equal(t, hash, "foo")
	})

	t.Run("Good Media Fingerprint", func(t *testing.T) {
		s := &sdp.SessionDescription{
			MediaDescriptions: []*sdp.MediaDescription{
				{Attributes: []sdp.Attribute{{Key: "fingerprint", Value: "foo bar"}}},
			},
		}

		fingerprint, hash, err := extractFingerprint(s)
		assert.NoError(t, err)
		assert.Equal(t, fingerprint, "bar")
		assert.Equal(t, hash, "foo")
	})

	t.Run("No Fingerprint", func(t *testing.T) {
		s := &sdp.SessionDescription{}

		_, _, err := extractFingerprint(s)
		assert.Equal(t, ErrSessionDescriptionNoFingerprint, err)
	})

	t.Run("Invalid Fingerprint", func(t *testing.T) {
		s := &sdp.SessionDescription{
			Attributes: []sdp.Attribute{{Key: "fingerprint", Value: "foo"}},
		}

		_, _, err := extractFingerprint(s)
		assert.Equal(t, ErrSessionDescriptionInvalidFingerprint, err)
	})

	t.Run("Conflicting Fingerprint", func(t *testing.T) {
		s := &sdp.SessionDescription{
			Attributes: []sdp.Attribute{{Key: "fingerprint", Value: "foo"}},
			MediaDescriptions: []*sdp.MediaDescription{
				{Attributes: []sdp.Attribute{{Key: "fingerprint", Value: "foo blah"}}},
			},
		}

		_, _, err := extractFingerprint(s)
		assert.Equal(t, ErrSessionDescriptionConflictingFingerprints, err)
	})
}

func TestExtractICEDetails(t *testing.T) {
	const defaultUfrag = "defaultPwd"
	const defaultPwd = "defaultUfrag"

	t.Run("Missing ice-pwd", func(t *testing.T) {
		s := &sdp.SessionDescription{
			MediaDescriptions: []*sdp.MediaDescription{
				{Attributes: []sdp.Attribute{{Key: "ice-ufrag", Value: defaultUfrag}}},
			},
		}

		_, _, _, err := extractICEDetails(s)
		assert.Equal(t, err, ErrSessionDescriptionMissingIcePwd)
	})

	t.Run("Missing ice-ufrag", func(t *testing.T) {
		s := &sdp.SessionDescription{
			MediaDescriptions: []*sdp.MediaDescription{
				{Attributes: []sdp.Attribute{{Key: "ice-pwd", Value: defaultPwd}}},
			},
		}

		_, _, _, err := extractICEDetails(s)
		assert.Equal(t, err, ErrSessionDescriptionMissingIceUfrag)
	})

	t.Run("ice details at session level", func(t *testing.T) {
		s := &sdp.SessionDescription{
			Attributes: []sdp.Attribute{
				{Key: "ice-ufrag", Value: defaultUfrag},
				{Key: "ice-pwd", Value: defaultPwd},
			},
			MediaDescriptions: []*sdp.MediaDescription{},
		}

		ufrag, pwd, _, err := extractICEDetails(s)
		assert.Equal(t, ufrag, defaultUfrag)
		assert.Equal(t, pwd, defaultPwd)
		assert.NoError(t, err)
	})

	t.Run("ice details at media level", func(t *testing.T) {
		s := &sdp.SessionDescription{
			MediaDescriptions: []*sdp.MediaDescription{
				{
					Attributes: []sdp.Attribute{
						{Key: "ice-ufrag", Value: defaultUfrag},
						{Key: "ice-pwd", Value: defaultPwd},
					},
				},
			},
		}

		ufrag, pwd, _, err := extractICEDetails(s)
		assert.Equal(t, ufrag, defaultUfrag)
		assert.Equal(t, pwd, defaultPwd)
		assert.NoError(t, err)
	})

	t.Run("Conflict ufrag", func(t *testing.T) {
		s := &sdp.SessionDescription{
			Attributes: []sdp.Attribute{{Key: "ice-ufrag", Value: "invalidUfrag"}},
			MediaDescriptions: []*sdp.MediaDescription{
				{Attributes: []sdp.Attribute{{Key: "ice-ufrag", Value: defaultUfrag}, {Key: "ice-pwd", Value: defaultPwd}}},
			},
		}

		_, _, _, err := extractICEDetails(s)
		assert.Equal(t, err, ErrSessionDescriptionConflictingIceUfrag)
	})

	t.Run("Conflict pwd", func(t *testing.T) {
		s := &sdp.SessionDescription{
			Attributes: []sdp.Attribute{{Key: "ice-pwd", Value: "invalidPwd"}},
			MediaDescriptions: []*sdp.MediaDescription{
				{Attributes: []sdp.Attribute{{Key: "ice-ufrag", Value: defaultUfrag}, {Key: "ice-pwd", Value: defaultPwd}}},
			},
		}

		_, _, _, err := extractICEDetails(s)
		assert.Equal(t, err, ErrSessionDescriptionConflictingIcePwd)
	})
}

func TestTrackDetailsFromSDP(t *testing.T) {
	t.Run("Tracks unknown, audio and video with RTX", func(t *testing.T) {
		s := &sdp.SessionDescription{
			MediaDescriptions: []*sdp.MediaDescription{
				{
					MediaName: sdp.MediaName{
						Media: "foobar",
					},
					Attributes: []sdp.Attribute{
						{Key: "mid", Value: "0"},
						{Key: "sendrecv"},
						{Key: "ssrc", Value: "1000 msid:unknown_trk_label unknown_trk_guid"},
					},
				},
				{
					MediaName: sdp.MediaName{
						Media: "audio",
					},
					Attributes: []sdp.Attribute{
						{Key: "mid", Value: "1"},
						{Key: "sendrecv"},
						{Key: "ssrc", Value: "2000 msid:audio_trk_label audio_trk_guid"},
					},
				},
				{
					MediaName: sdp.MediaName{
						Media: "video",
					},
					Attributes: []sdp.Attribute{
						{Key: "mid", Value: "2"},
						{Key: "sendrecv"},
						{Key: "ssrc-group", Value: "FID 3000 4000"},
						{Key: "ssrc", Value: "3000 msid:video_trk_label video_trk_guid"},
						{Key: "ssrc", Value: "4000 msid:rtx_trk_label rtx_trck_guid"},
					},
				},
				{
					MediaName: sdp.MediaName{
						Media: "video",
					},
					Attributes: []sdp.Attribute{
						{Key: "mid", Value: "3"},
						{Key: "sendonly"},
						{Key: "msid", Value: "video_stream_id video_trk_id"},
						{Key: "ssrc", Value: "5000"},
					},
				},
				{
					MediaName: sdp.MediaName{
						Media: "video",
					},
					Attributes: []sdp.Attribute{
						{Key: "sendonly"},
						{Key: "rid", Value: "f send pt=97;max-width=1280;max-height=720"},
					},
				},
			},
		}

		tracks := trackDetailsFromSDP(nil, s)
		assert.Equal(t, 3, len(tracks))
		if trackDetail := trackDetailsForSSRC(tracks, 1000); trackDetail != nil {
			assert.Fail(t, "got the unknown track ssrc:1000 which should have been skipped")
		}
		if track := trackDetailsForSSRC(tracks, 2000); track == nil {
			assert.Fail(t, "missing audio track with ssrc:2000")
		} else {
			assert.Equal(t, RTPCodecTypeAudio, track.kind)
			assert.Equal(t, SSRC(2000), track.ssrc)
			assert.Equal(t, "audio_trk_label", track.streamID)
		}
		if track := trackDetailsForSSRC(tracks, 3000); track == nil {
			assert.Fail(t, "missing video track with ssrc:3000")
		} else {
			assert.Equal(t, RTPCodecTypeVideo, track.kind)
			assert.Equal(t, SSRC(3000), track.ssrc)
			assert.Equal(t, "video_trk_label", track.streamID)
		}
		if track := trackDetailsForSSRC(tracks, 4000); track != nil {
			assert.Fail(t, "got the rtx track ssrc:3000 which should have been skipped")
		}
		if track := trackDetailsForSSRC(tracks, 5000); track == nil {
			assert.Fail(t, "missing video track with ssrc:5000")
		} else {
			assert.Equal(t, RTPCodecTypeVideo, track.kind)
			assert.Equal(t, SSRC(5000), track.ssrc)
			assert.Equal(t, "video_trk_id", track.id)
			assert.Equal(t, "video_stream_id", track.streamID)
		}
	})

	t.Run("inactive and recvonly tracks ignored", func(t *testing.T) {
		s := &sdp.SessionDescription{
			MediaDescriptions: []*sdp.MediaDescription{
				{
					MediaName: sdp.MediaName{
						Media: "video",
					},
					Attributes: []sdp.Attribute{
						{Key: "inactive"},
						{Key: "ssrc", Value: "6000"},
					},
				},
				{
					MediaName: sdp.MediaName{
						Media: "video",
					},
					Attributes: []sdp.Attribute{
						{Key: "recvonly"},
						{Key: "ssrc", Value: "7000"},
					},
				},
			},
		}
		assert.Equal(t, 0, len(trackDetailsFromSDP(nil, s)))
	})
}

func TestHaveApplicationMediaSection(t *testing.T) {
	t.Run("Audio only", func(t *testing.T) {
		s := &sdp.SessionDescription{
			MediaDescriptions: []*sdp.MediaDescription{
				{
					MediaName: sdp.MediaName{
						Media: "audio",
					},
					Attributes: []sdp.Attribute{
						{Key: "sendrecv"},
						{Key: "ssrc", Value: "2000"},
					},
				},
			},
		}

		assert.False(t, haveApplicationMediaSection(s))
	})

	t.Run("Application", func(t *testing.T) {
		s := &sdp.SessionDescription{
			MediaDescriptions: []*sdp.MediaDescription{
				{
					MediaName: sdp.MediaName{
						Media: mediaSectionApplication,
					},
				},
			},
		}

		assert.True(t, haveApplicationMediaSection(s))
	})
}

func TestMediaDescriptionFingerprints(t *testing.T) {
	engine := &MediaEngine{}
	assert.NoError(t, engine.RegisterDefaultCodecs())

	sk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	assert.NoError(t, err)

	certificate, err := GenerateCertificate(sk)
	assert.NoError(t, err)

	media := []mediaSection{
		{
			id: "video",
			transceivers: []*RTPTransceiver{{
				kind: RTPCodecTypeVideo,
			}},
		},
		{
			id: "audio",
			transceivers: []*RTPTransceiver{{
				kind: RTPCodecTypeAudio,
			}},
		},
		{
			id:   "application",
			data: true,
		},
	}

	for i := 0; i < 2; i++ {
		media[i].transceivers[0].setSender(&RTPSender{})
		media[i].transceivers[0].setDirection(RTPTransceiverDirectionSendonly)
	}

	fingerprintTest := func(SDPMediaDescriptionFingerprints bool, expectedFingerprintCount int) func(t *testing.T) {
		return func(t *testing.T) {
			s := &sdp.SessionDescription{}

			dtlsFingerprints, err := certificate.GetFingerprints()
			assert.NoError(t, err)

			s, err = populateSDP(s, false,
				dtlsFingerprints,
				SDPMediaDescriptionFingerprints,
				false, engine, sdp.ConnectionRoleActive, []ICECandidate{}, ICEParameters{}, media, ICEGatheringStateNew)
			assert.NoError(t, err)

			sdparray, err := s.Marshal()
			assert.NoError(t, err)

			assert.Equal(t, strings.Count(string(sdparray), "sha-256"), expectedFingerprintCount)
		}
	}

	t.Run("Per-Media Description Fingerprints", fingerprintTest(true, 3))
	t.Run("Per-Session Description Fingerprints", fingerprintTest(false, 1))
}

func TestPopulateSDP(t *testing.T) {
	t.Run("Rid", func(t *testing.T) {
		tr := newRTPTransceiver(nil, nil, RTPTransceiverDirectionRecvonly, RTPCodecTypeVideo)
		ridMap := map[string]string{
			"ridkey": "some",
		}
		mediaSections := []mediaSection{{id: "video", transceivers: []*RTPTransceiver{tr}, ridMap: ridMap}}

		se := SettingEngine{}

		m := MediaEngine{}
		assert.NoError(t, m.RegisterDefaultCodecs())

		d := &sdp.SessionDescription{}

		offerSdp, err := populateSDP(d, false, []DTLSFingerprint{}, se.sdpMediaLevelFingerprints, se.candidates.ICELite, &m, connectionRoleFromDtlsRole(defaultDtlsRoleOffer), []ICECandidate{}, ICEParameters{}, mediaSections, ICEGatheringStateComplete)
		assert.Nil(t, err)

		// Test contains rid map keys
		var found bool
		for _, desc := range offerSdp.MediaDescriptions {
			if desc.MediaName.Media != "video" {
				continue
			}
			for _, a := range desc.Attributes {
				if a.Key == "rid" {
					if strings.Contains(a.Value, "ridkey") {
						found = true
						break
					}
				}
			}
		}
		assert.Equal(t, true, found, "Rid key should be present")
	})
}

func TestGetRIDs(t *testing.T) {
	m := []*sdp.MediaDescription{
		{
			MediaName: sdp.MediaName{
				Media: "video",
			},
			Attributes: []sdp.Attribute{
				{Key: "sendonly"},
				{Key: "rid", Value: "f send pt=97;max-width=1280;max-height=720"},
			},
		},
	}

	rids := getRids(m[0])

	assert.NotEmpty(t, rids, "Rid mapping should be present")
	if _, ok := rids["f"]; !ok {
		assert.Fail(t, "rid values should contain 'f'")
	}
}

func TestCodecsFromMediaDescription(t *testing.T) {
	t.Run("Codec Only", func(t *testing.T) {
		codecs, err := codecsFromMediaDescription(&sdp.MediaDescription{
			MediaName: sdp.MediaName{
				Media:   "audio",
				Formats: []string{"111"},
			},
			Attributes: []sdp.Attribute{
				{Key: "rtpmap", Value: "111 opus/48000/2"},
			},
		})

		assert.Equal(t, codecs, []RTPCodecParameters{
			{
				RTPCodecCapability: RTPCodecCapability{MimeTypeOpus, 48000, 2, "", []RTCPFeedback{}},
				PayloadType:        111,
			},
		})
		assert.NoError(t, err)
	})

	t.Run("Codec with fmtp/rtcp-fb", func(t *testing.T) {
		codecs, err := codecsFromMediaDescription(&sdp.MediaDescription{
			MediaName: sdp.MediaName{
				Media:   "audio",
				Formats: []string{"111"},
			},
			Attributes: []sdp.Attribute{
				{Key: "rtpmap", Value: "111 opus/48000/2"},
				{Key: "fmtp", Value: "111 minptime=10;useinbandfec=1"},
				{Key: "rtcp-fb", Value: "111 goog-remb"},
				{Key: "rtcp-fb", Value: "111 ccm fir"},
			},
		})

		assert.Equal(t, codecs, []RTPCodecParameters{
			{
				RTPCodecCapability: RTPCodecCapability{MimeTypeOpus, 48000, 2, "minptime=10;useinbandfec=1", []RTCPFeedback{{"goog-remb", ""}, {"ccm", "fir"}}},
				PayloadType:        111,
			},
		})
		assert.NoError(t, err)
	})
}

func TestRtpExtensionsFromMediaDescription(t *testing.T) {
	extensions, err := rtpExtensionsFromMediaDescription(&sdp.MediaDescription{
		MediaName: sdp.MediaName{
			Media:   "audio",
			Formats: []string{"111"},
		},
		Attributes: []sdp.Attribute{
			{Key: "extmap", Value: "1 " + sdp.ABSSendTimeURI},
			{Key: "extmap", Value: "3 " + sdp.SDESMidURI},
		},
	})

	assert.NoError(t, err)
	assert.Equal(t, extensions[sdp.ABSSendTimeURI], 1)
	assert.Equal(t, extensions[sdp.SDESMidURI], 3)
}
