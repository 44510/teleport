/*
Copyright 2021 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package desktop

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/teleport/api/types/events"
	libevents "github.com/gravitational/teleport/lib/events"
	"github.com/gravitational/teleport/lib/events/eventstest"
	"github.com/gravitational/teleport/lib/srv/desktop/tdp"
	"github.com/gravitational/teleport/lib/tlsca"
	"github.com/gravitational/trace"
	"github.com/jonboulle/clockwork"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func setup() (*WindowsService, *tlsca.Identity, *eventstest.MockEmitter) {
	emitter := &eventstest.MockEmitter{}
	log := logrus.New()
	log.SetOutput(io.Discard)

	s := &WindowsService{
		clusterName: "test-cluster",
		cfg: WindowsServiceConfig{
			Log:     log,
			Emitter: emitter,
			Heartbeat: HeartbeatConfig{
				HostUUID: "test-host-id",
			},
			Clock: clockwork.NewFakeClockAt(time.Now()),
		},
		auditCache: newSharedDirectoryAuditCache(),
	}

	id := &tlsca.Identity{
		Username:     "foo",
		Impersonator: "bar",
		MFAVerified:  "mfa-id",
		ClientIP:     "127.0.0.1",
	}

	return s, id, emitter
}

func TestSessionStartEvent(t *testing.T) {
	s, id, emitter := setup()

	desktop := &types.WindowsDesktopV3{
		ResourceHeader: types.ResourceHeader{
			Metadata: types.Metadata{
				Name:   "test-desktop",
				Labels: map[string]string{"env": "production"},
			},
		},
		Spec: types.WindowsDesktopSpecV3{
			Addr:   "192.168.100.12",
			Domain: "test.example.com",
		},
	}

	userMeta := id.GetUserMetadata()
	userMeta.Login = "Administrator"
	expected := &events.WindowsDesktopSessionStart{
		Metadata: events.Metadata{
			ClusterName: s.clusterName,
			Type:        libevents.WindowsDesktopSessionStartEvent,
			Code:        libevents.DesktopSessionStartCode,
			Time:        s.cfg.Clock.Now().UTC().Round(time.Millisecond),
		},
		UserMetadata: userMeta,
		SessionMetadata: events.SessionMetadata{
			SessionID: "sessionID",
			WithMFA:   id.MFAVerified,
		},
		ConnectionMetadata: events.ConnectionMetadata{
			LocalAddr:  id.ClientIP,
			RemoteAddr: desktop.GetAddr(),
			Protocol:   libevents.EventProtocolTDP,
		},
		Status: events.Status{
			Success: true,
		},
		WindowsDesktopService: s.cfg.Heartbeat.HostUUID,
		DesktopName:           "test-desktop",
		DesktopAddr:           desktop.GetAddr(),
		Domain:                desktop.GetDomain(),
		WindowsUser:           "Administrator",
		DesktopLabels:         map[string]string{"env": "production"},
	}

	for _, test := range []struct {
		desc string
		err  error
		exp  func() events.WindowsDesktopSessionStart
	}{
		{
			desc: "success",
			err:  nil,
			exp:  func() events.WindowsDesktopSessionStart { return *expected },
		},
		{
			desc: "failure",
			err:  trace.AccessDenied("access denied"),
			exp: func() events.WindowsDesktopSessionStart {
				e := *expected
				e.Code = libevents.DesktopSessionStartFailureCode
				e.Success = false
				e.Error = "access denied"
				e.UserMessage = "access denied"
				return e
			},
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			s.onSessionStart(
				context.Background(),
				s.cfg.Emitter,
				id,
				s.cfg.Clock.Now().UTC().Round(time.Millisecond),
				"Administrator",
				"sessionID",
				desktop,
				test.err,
			)

			event := emitter.LastEvent()
			require.NotNil(t, event)

			startEvent, ok := event.(*events.WindowsDesktopSessionStart)
			require.True(t, ok)

			require.Empty(t, cmp.Diff(test.exp(), *startEvent))
		})
	}
}

func TestSessionEndEvent(t *testing.T) {
	s, id, emitter := setup()

	desktop := &types.WindowsDesktopV3{
		ResourceHeader: types.ResourceHeader{
			Metadata: types.Metadata{
				Name:   "test-desktop",
				Labels: map[string]string{"env": "production"},
			},
		},
		Spec: types.WindowsDesktopSpecV3{
			Addr:   "192.168.100.12",
			Domain: "test.example.com",
		},
	}

	c := clockwork.NewFakeClockAt(time.Now())
	s.cfg.Clock = c
	startTime := s.cfg.Clock.Now().UTC().Round(time.Millisecond)
	c.Advance(30 * time.Second)

	s.onSessionEnd(
		context.Background(),
		s.cfg.Emitter,
		id,
		startTime,
		true,
		"Administrator",
		"sessionID",
		desktop,
	)

	event := emitter.LastEvent()
	require.NotNil(t, event)
	endEvent, ok := event.(*events.WindowsDesktopSessionEnd)
	require.True(t, ok)

	userMeta := id.GetUserMetadata()
	userMeta.Login = "Administrator"
	expected := &events.WindowsDesktopSessionEnd{
		Metadata: events.Metadata{
			ClusterName: s.clusterName,
			Type:        libevents.WindowsDesktopSessionEndEvent,
			Code:        libevents.DesktopSessionEndCode,
		},
		UserMetadata: userMeta,
		SessionMetadata: events.SessionMetadata{
			SessionID: "sessionID",
			WithMFA:   id.MFAVerified,
		},
		WindowsDesktopService: s.cfg.Heartbeat.HostUUID,
		DesktopAddr:           desktop.GetAddr(),
		Domain:                desktop.GetDomain(),
		WindowsUser:           "Administrator",
		DesktopLabels:         map[string]string{"env": "production"},
		StartTime:             startTime,
		EndTime:               c.Now().UTC().Round(time.Millisecond),
		DesktopName:           desktop.GetName(),
		Recorded:              true,
		Participants:          []string{"foo"},
	}
	require.Empty(t, cmp.Diff(expected, endEvent))
}

func TestDesktopSharedDirectoryStartEvent(t *testing.T) {
	sid := "session-0"
	desktopAddr := "windows.example.com"
	testDirName := "test-dir"
	var did uint32 = 2

	for _, test := range []struct {
		name string
		// sendsSda determines whether a SharedDirectoryAnnounce is sent.
		sendsSda bool
		// errCode is the error code in the simulated SharedDirectoryAcknowledge
		errCode uint32
		// expected returns the event we expect to be emitted by modifying baseEvent
		// (which is passed in from the test body below).
		expected func(baseEvent *events.DesktopSharedDirectoryStart) *events.DesktopSharedDirectoryStart
	}{
		{
			// when everything is working as expected
			name:     "typical operation",
			sendsSda: true,
			errCode:  tdp.ErrCodeNil,
			expected: func(baseEvent *events.DesktopSharedDirectoryStart) *events.DesktopSharedDirectoryStart {
				return baseEvent
			},
		},
		{
			// the announce operation failed
			name:     "announce failed",
			sendsSda: true,
			errCode:  tdp.ErrCodeFailed,
			expected: func(baseEvent *events.DesktopSharedDirectoryStart) *events.DesktopSharedDirectoryStart {
				baseEvent.Metadata.Code = libevents.DesktopSharedDirectoryStartFailureCode
				return baseEvent
			},
		},
		{
			// should never happen but just in case
			name:     "directory name unknown",
			sendsSda: false,
			errCode:  tdp.ErrCodeNil,
			expected: func(baseEvent *events.DesktopSharedDirectoryStart) *events.DesktopSharedDirectoryStart {
				baseEvent.Metadata.Code = libevents.DesktopSharedDirectoryStartFailureCode
				baseEvent.DirectoryName = "unknown"
				return baseEvent
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, id, emitter := setup()
			recvHandler := s.makeTDPReceiveHandler(context.Background(),
				emitter, func() int64 { return 0 },
				id, sid, desktopAddr)
			sendHandler := s.makeTDPSendHandler(context.Background(),
				emitter, func() int64 { return 0 },
				id, sid, desktopAddr)

			if test.sendsSda {
				// SharedDirectoryAnnounce initializes the nameCache.
				sda := tdp.SharedDirectoryAnnounce{
					DirectoryID: did,
					Name:        testDirName,
				}
				recvHandler(sda)
			}

			// SharedDirectoryAcknowledge causes the event to be emitted
			// (or not, on failure).
			ack := tdp.SharedDirectoryAcknowledge{
				DirectoryID: did,
				ErrCode:     test.errCode,
			}
			encoded, err := ack.Encode()
			require.NoError(t, err)
			sendHandler(ack, encoded)

			baseEvent := &events.DesktopSharedDirectoryStart{
				Metadata: events.Metadata{
					Type:        libevents.DesktopSharedDirectoryStartEvent,
					Code:        libevents.DesktopSharedDirectoryStartCode,
					ClusterName: s.clusterName,
					Time:        s.cfg.Clock.Now().UTC(),
				},
				UserMetadata: id.GetUserMetadata(),
				SessionMetadata: events.SessionMetadata{
					SessionID: sid,
					WithMFA:   id.MFAVerified,
				},
				ConnectionMetadata: events.ConnectionMetadata{
					LocalAddr:  id.ClientIP,
					RemoteAddr: desktopAddr,
					Protocol:   libevents.EventProtocolTDP,
				},
				DesktopAddr:   desktopAddr,
				DirectoryName: testDirName,
				DirectoryID:   did,
			}

			expected := test.expected(baseEvent)
			event := emitter.LastEvent()

			require.NotNil(t, event)
			startEvent, ok := event.(*events.DesktopSharedDirectoryStart)
			require.True(t, ok)

			require.Empty(t, cmp.Diff(expected, startEvent))
		})
	}
}

func TestDesktopSharedDirectoryReadEvent(t *testing.T) {
	sid := "session-0"
	desktopAddr := "windows.example.com"
	testDirName := "test-dir"
	path := "test/path/test-file.txt"
	var did uint32 = 2
	var cid uint32 = 999
	var offset uint64 = 500
	var length uint32 = 1000

	for _, test := range []struct {
		name string
		// sendsSda determines whether a SharedDirectoryAnnounce is sent.
		sendsSda bool
		// sendsReq determines whether a SharedDirectoryReadRequest is sent.
		sendsReq bool
		// errCode is the error code in the simulated SharedDirectoryReadResponse
		errCode uint32
		// expected returns the event we expect to be emitted by modifying baseEvent
		// (which is passed in from the test body below).
		expected func(baseEvent *events.DesktopSharedDirectoryRead) *events.DesktopSharedDirectoryRead
	}{
		{
			// when everything is working as expected
			name:     "typical operation",
			sendsSda: true,
			sendsReq: true,
			errCode:  tdp.ErrCodeNil,
			expected: func(baseEvent *events.DesktopSharedDirectoryRead) *events.DesktopSharedDirectoryRead {
				return baseEvent
			},
		},
		{
			// the read operation failed
			name:     "read failed",
			sendsSda: true,
			sendsReq: true,
			errCode:  tdp.ErrCodeFailed,
			expected: func(baseEvent *events.DesktopSharedDirectoryRead) *events.DesktopSharedDirectoryRead {
				baseEvent.Metadata.Code = libevents.DesktopSharedDirectoryWriteFailureCode
				return baseEvent
			},
		},
		{
			// should never happen but just in case
			name:     "directory name unknown",
			sendsSda: false,
			sendsReq: true,
			errCode:  tdp.ErrCodeNil,
			expected: func(baseEvent *events.DesktopSharedDirectoryRead) *events.DesktopSharedDirectoryRead {
				baseEvent.Metadata.Code = libevents.DesktopSharedDirectoryReadFailureCode
				baseEvent.DirectoryName = "unknown"
				return baseEvent
			},
		},
		{
			// should never happen but just in case
			name:     "request info unknown",
			sendsSda: true,
			sendsReq: false,
			errCode:  tdp.ErrCodeNil,
			expected: func(baseEvent *events.DesktopSharedDirectoryRead) *events.DesktopSharedDirectoryRead {
				baseEvent.Metadata.Code = libevents.DesktopSharedDirectoryReadFailureCode

				// resorts to default values for these
				baseEvent.DirectoryID = 0
				baseEvent.Offset = 0

				// sets "unknown" for these
				baseEvent.Path = "unknown"
				// we can't retrieve the directory name because we don't have the directoryID
				baseEvent.DirectoryName = "unknown"

				return baseEvent
			},
		},
		{
			// should never happen but just in case
			name:     "directory name and request info unknown",
			sendsSda: false,
			sendsReq: false,
			errCode:  tdp.ErrCodeNil,
			expected: func(baseEvent *events.DesktopSharedDirectoryRead) *events.DesktopSharedDirectoryRead {
				baseEvent.Metadata.Code = libevents.DesktopSharedDirectoryReadFailureCode

				// resorts to default values for these
				baseEvent.DirectoryID = 0
				baseEvent.Offset = 0

				// sets "unknown" for these
				baseEvent.Path = "unknown"
				baseEvent.DirectoryName = "unknown"

				return baseEvent
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, id, emitter := setup()
			recvHandler := s.makeTDPReceiveHandler(context.Background(),
				emitter, func() int64 { return 0 },
				id, sid, desktopAddr)
			sendHandler := s.makeTDPSendHandler(context.Background(),
				emitter, func() int64 { return 0 },
				id, sid, desktopAddr)
			if test.sendsSda {
				// SharedDirectoryAnnounce initializes the nameCache.
				sda := tdp.SharedDirectoryAnnounce{
					DirectoryID: did,
					Name:        testDirName,
				}
				recvHandler(sda)
			}

			if test.sendsReq {
				// SharedDirectoryReadRequest initializes the readRequestCache.
				req := tdp.SharedDirectoryReadRequest{
					CompletionID: cid,
					DirectoryID:  did,
					Path:         path,
					Offset:       offset,
					Length:       length,
				}
				encoded, err := req.Encode()
				require.NoError(t, err)
				sendHandler(req, encoded)
			}

			// SharedDirectoryReadResponse causes the event to be emitted.
			res := tdp.SharedDirectoryReadResponse{
				CompletionID:   cid,
				ErrCode:        test.errCode,
				ReadDataLength: length,
				ReadData:       []byte{}, // irrelevant in this context
			}
			recvHandler(res)

			event := emitter.LastEvent()
			require.NotNil(t, event)

			readEvent, ok := event.(*events.DesktopSharedDirectoryRead)
			require.True(t, ok)

			baseEvent := &events.DesktopSharedDirectoryRead{
				Metadata: events.Metadata{
					Type:        libevents.DesktopSharedDirectoryReadEvent,
					Code:        libevents.DesktopSharedDirectoryReadCode,
					ClusterName: s.clusterName,
					Time:        s.cfg.Clock.Now().UTC(),
				},
				UserMetadata: id.GetUserMetadata(),
				SessionMetadata: events.SessionMetadata{
					SessionID: sid,
					WithMFA:   id.MFAVerified,
				},
				ConnectionMetadata: events.ConnectionMetadata{
					LocalAddr:  id.ClientIP,
					RemoteAddr: desktopAddr,
					Protocol:   libevents.EventProtocolTDP,
				},
				DesktopAddr:   desktopAddr,
				DirectoryName: testDirName,
				DirectoryID:   did,
				Path:          path,
				Length:        length,
				Offset:        offset,
			}

			require.Empty(t, cmp.Diff(test.expected(baseEvent), readEvent))
		})
	}
}

func TestDesktopSharedDirectoryWriteEvent(t *testing.T) {
	sid := "session-0"
	desktopAddr := "windows.example.com"
	testDirName := "test-dir"
	path := "test/path/test-file.txt"
	var did uint32 = 2
	var cid uint32 = 999
	var offset uint64 = 500
	var length uint32 = 1000

	for _, test := range []struct {
		name string
		// sendsSda determines whether a SharedDirectoryAnnounce is sent.
		sendsSda bool
		// sendsReq determines whether a SharedDirectoryWriteRequest is sent.
		sendsReq bool
		// errCode is the error code in the simulated SharedDirectoryWriteResponse
		errCode uint32
		// expected returns the event we expect to be emitted by modifying baseEvent
		// (which is passed in from the test body below).
		expected func(baseEvent *events.DesktopSharedDirectoryWrite) *events.DesktopSharedDirectoryWrite
	}{
		{
			// when everything is working as expected
			name:     "typical operation",
			sendsSda: true,
			sendsReq: true,
			errCode:  tdp.ErrCodeNil,
			expected: func(baseEvent *events.DesktopSharedDirectoryWrite) *events.DesktopSharedDirectoryWrite {
				return baseEvent
			},
		},
		{
			// the Write operation failed
			name:     "write failed",
			sendsSda: true,
			sendsReq: true,
			errCode:  tdp.ErrCodeFailed,
			expected: func(baseEvent *events.DesktopSharedDirectoryWrite) *events.DesktopSharedDirectoryWrite {
				baseEvent.Metadata.Code = libevents.DesktopSharedDirectoryWriteFailureCode
				return baseEvent
			},
		},
		{
			// should never happen but just in case
			name:     "directory name unknown",
			sendsSda: false,
			sendsReq: true,
			errCode:  tdp.ErrCodeNil,
			expected: func(baseEvent *events.DesktopSharedDirectoryWrite) *events.DesktopSharedDirectoryWrite {
				baseEvent.Metadata.Code = libevents.DesktopSharedDirectoryWriteFailureCode
				baseEvent.DirectoryName = "unknown"
				return baseEvent
			},
		},
		{
			// should never happen but just in case
			name:     "request info unknown",
			sendsSda: true,
			sendsReq: false,
			errCode:  tdp.ErrCodeNil,
			expected: func(baseEvent *events.DesktopSharedDirectoryWrite) *events.DesktopSharedDirectoryWrite {
				baseEvent.Metadata.Code = libevents.DesktopSharedDirectoryWriteFailureCode

				// resorts to default values for these
				baseEvent.DirectoryID = 0
				baseEvent.Offset = 0

				// sets "unknown" for these
				baseEvent.Path = "unknown"
				// we can't retrieve the directory name because we don't have the directoryID
				baseEvent.DirectoryName = "unknown"

				return baseEvent
			},
		},
		{
			// should never happen but just in case
			name:     "directory name and request info unknown",
			sendsSda: false,
			sendsReq: false,
			errCode:  tdp.ErrCodeNil,
			expected: func(baseEvent *events.DesktopSharedDirectoryWrite) *events.DesktopSharedDirectoryWrite {
				baseEvent.Metadata.Code = libevents.DesktopSharedDirectoryWriteFailureCode

				// resorts to default values for these
				baseEvent.DirectoryID = 0
				baseEvent.Offset = 0

				// sets "unknown" for these
				baseEvent.Path = "unknown"
				baseEvent.DirectoryName = "unknown"

				return baseEvent
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, id, emitter := setup()
			recvHandler := s.makeTDPReceiveHandler(context.Background(),
				emitter, func() int64 { return 0 },
				id, sid, desktopAddr)
			sendHandler := s.makeTDPSendHandler(context.Background(),
				emitter, func() int64 { return 0 },
				id, sid, desktopAddr)
			if test.sendsSda {
				// SharedDirectoryAnnounce initializes the nameCache.
				sda := tdp.SharedDirectoryAnnounce{
					DirectoryID: did,
					Name:        testDirName,
				}
				recvHandler(sda)
			}

			if test.sendsReq {
				// SharedDirectoryWriteRequest initializes the writeRequestCache.
				req := tdp.SharedDirectoryWriteRequest{
					CompletionID:    cid,
					DirectoryID:     did,
					Path:            path,
					Offset:          offset,
					WriteDataLength: length,
				}
				encoded, err := req.Encode()
				require.NoError(t, err)
				sendHandler(req, encoded)
			}

			// SharedDirectoryWriteResponse causes the event to be emitted.
			res := tdp.SharedDirectoryWriteResponse{
				CompletionID: cid,
				ErrCode:      test.errCode,
				BytesWritten: length,
			}
			recvHandler(res)

			event := emitter.LastEvent()
			require.NotNil(t, event)

			writeEvent, ok := event.(*events.DesktopSharedDirectoryWrite)
			require.True(t, ok)

			baseEvent := &events.DesktopSharedDirectoryWrite{
				Metadata: events.Metadata{
					Type:        libevents.DesktopSharedDirectoryWriteEvent,
					Code:        libevents.DesktopSharedDirectoryWriteCode,
					ClusterName: s.clusterName,
					Time:        s.cfg.Clock.Now().UTC(),
				},
				UserMetadata: id.GetUserMetadata(),
				SessionMetadata: events.SessionMetadata{
					SessionID: sid,
					WithMFA:   id.MFAVerified,
				},
				ConnectionMetadata: events.ConnectionMetadata{
					LocalAddr:  id.ClientIP,
					RemoteAddr: desktopAddr,
					Protocol:   libevents.EventProtocolTDP,
				},
				DesktopAddr:   desktopAddr,
				DirectoryName: testDirName,
				DirectoryID:   did,
				Path:          path,
				Length:        length,
				Offset:        offset,
			}

			require.Empty(t, cmp.Diff(test.expected(baseEvent), writeEvent))
		})
	}
}
