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
	"time"

	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/teleport/api/types/events"
	libevents "github.com/gravitational/teleport/lib/events"
	"github.com/gravitational/teleport/lib/srv/desktop/tdp"
	"github.com/gravitational/teleport/lib/tlsca"
	"github.com/gravitational/trace"
)

func (s *WindowsService) onSessionStart(ctx context.Context, emitter events.Emitter, id *tlsca.Identity, startTime time.Time, windowsUser, sessionID string, desktop types.WindowsDesktop, err error) {
	userMetadata := id.GetUserMetadata()
	userMetadata.Login = windowsUser

	event := &events.WindowsDesktopSessionStart{
		Metadata: events.Metadata{
			Type:        libevents.WindowsDesktopSessionStartEvent,
			Code:        libevents.DesktopSessionStartCode,
			ClusterName: s.clusterName,
			Time:        startTime,
		},
		UserMetadata: userMetadata,
		SessionMetadata: events.SessionMetadata{
			SessionID: sessionID,
			WithMFA:   id.MFAVerified,
		},
		ConnectionMetadata: events.ConnectionMetadata{
			LocalAddr:  id.ClientIP,
			RemoteAddr: desktop.GetAddr(),
			Protocol:   libevents.EventProtocolTDP,
		},
		Status: events.Status{
			Success: err == nil,
		},
		WindowsDesktopService: s.cfg.Heartbeat.HostUUID,
		DesktopName:           desktop.GetName(),
		DesktopAddr:           desktop.GetAddr(),
		Domain:                desktop.GetDomain(),
		WindowsUser:           windowsUser,
		DesktopLabels:         desktop.GetAllLabels(),
	}
	if err != nil {
		event.Code = libevents.DesktopSessionStartFailureCode
		event.Error = trace.Unwrap(err).Error()
		event.UserMessage = err.Error()
	}
	s.emit(ctx, emitter, event)
}

func (s *WindowsService) onSessionEnd(ctx context.Context, emitter events.Emitter, id *tlsca.Identity, startedAt time.Time, recorded bool, windowsUser, sid string, desktop types.WindowsDesktop) {
	// Ensure audit cache gets cleaned up
	s.auditCache.Delete(sessionID(sid))

	userMetadata := id.GetUserMetadata()
	userMetadata.Login = windowsUser

	event := &events.WindowsDesktopSessionEnd{
		Metadata: events.Metadata{
			Type:        libevents.WindowsDesktopSessionEndEvent,
			Code:        libevents.DesktopSessionEndCode,
			ClusterName: s.clusterName,
		},
		UserMetadata: userMetadata,
		SessionMetadata: events.SessionMetadata{
			SessionID: sid,
			WithMFA:   id.MFAVerified,
		},
		WindowsDesktopService: s.cfg.Heartbeat.HostUUID,
		DesktopAddr:           desktop.GetAddr(),
		Domain:                desktop.GetDomain(),
		WindowsUser:           windowsUser,
		DesktopLabels:         desktop.GetAllLabels(),
		StartTime:             startedAt,
		EndTime:               s.cfg.Clock.Now().UTC().Round(time.Millisecond),
		DesktopName:           desktop.GetName(),
		Recorded:              recorded,

		// There can only be 1 participant, desktop sessions are not join-able.
		Participants: []string{userMetadata.User},
	}
	s.emit(ctx, emitter, event)
}

func (s *WindowsService) onClipboardSend(ctx context.Context, emitter events.Emitter, id *tlsca.Identity, sessionID string, desktopAddr string, length int32) {
	event := &events.DesktopClipboardSend{
		Metadata: events.Metadata{
			Type:        libevents.DesktopClipboardSendEvent,
			Code:        libevents.DesktopClipboardSendCode,
			ClusterName: s.clusterName,
			Time:        s.cfg.Clock.Now().UTC(),
		},
		UserMetadata: id.GetUserMetadata(),
		SessionMetadata: events.SessionMetadata{
			SessionID: sessionID,
			WithMFA:   id.MFAVerified,
		},
		ConnectionMetadata: events.ConnectionMetadata{
			LocalAddr:  id.ClientIP,
			RemoteAddr: desktopAddr,
			Protocol:   libevents.EventProtocolTDP,
		},
		DesktopAddr: desktopAddr,
		Length:      length,
	}
	s.emit(ctx, emitter, event)
}

func (s *WindowsService) onClipboardReceive(ctx context.Context, emitter events.Emitter, id *tlsca.Identity, sessionID string, desktopAddr string, length int32) {
	event := &events.DesktopClipboardReceive{
		Metadata: events.Metadata{
			Type:        libevents.DesktopClipboardReceiveEvent,
			Code:        libevents.DesktopClipboardReceiveCode,
			ClusterName: s.clusterName,
			Time:        s.cfg.Clock.Now().UTC(),
		},
		UserMetadata: id.GetUserMetadata(),
		SessionMetadata: events.SessionMetadata{
			SessionID: sessionID,
			WithMFA:   id.MFAVerified,
		},
		ConnectionMetadata: events.ConnectionMetadata{
			LocalAddr:  id.ClientIP,
			RemoteAddr: desktopAddr,
			Protocol:   libevents.EventProtocolTDP,
		},
		DesktopAddr: desktopAddr,
		Length:      length,
	}
	s.emit(ctx, emitter, event)
}

// onSharedDirectoryAnnounce adds the shared directory's name to the auditCache.
func (s *WindowsService) onSharedDirectoryAnnounce(sid string, m tdp.SharedDirectoryAnnounce) {
	s.auditCache.SetName(sessionID(sid), directoryID(m.DirectoryID), directoryName(m.Name))
}

// onSharedDirectoryAcknowledge emits a DesktopSharedDirectoryStart on a successful receipt of a
// successful tdp.SharedDirectoryAcknowledge.
func (s *WindowsService) onSharedDirectoryAcknowledge(
	ctx context.Context,
	emitter events.Emitter,
	id *tlsca.Identity,
	sid string,
	desktopAddr string,
	m tdp.SharedDirectoryAcknowledge,
) {
	if m.ErrCode != tdp.ErrCodeNil {
		return
	}

	code := libevents.DesktopSharedDirectoryStartCode
	name, ok := s.auditCache.GetName(sessionID(sid), directoryID(m.DirectoryID))
	if !ok {
		code = libevents.DesktopSharedDirectoryStartCorruptedCode
		name = events.UnknownEvent
		s.cfg.Log.Warnf("failed to find a directory name corresponding to sessionID(%v), directoryID(%v)", sid, m.DirectoryID)
	}

	event := &events.DesktopSharedDirectoryStart{
		Metadata: events.Metadata{
			Type:        libevents.DesktopSharedDirectoryStartEvent,
			Code:        code,
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
		DirectoryName: string(name),
		DirectoryID:   m.DirectoryID,
	}

	s.emit(ctx, emitter, event)
}

// onSharedDirectoryReadRequest adds ReadRequestInfo to the auditCache.
func (s *WindowsService) onSharedDirectoryReadRequest(sid string, m tdp.SharedDirectoryReadRequest) {
	s.auditCache.SetReadRequestInfo(sessionID(sid), completionID(m.CompletionID), readRequestInfo{
		directoryID: directoryID(m.DirectoryID),
		path:        m.Path,
		offset:      m.Offset,
	})
}

// onSharedDirectoryReadResponse emits a DesktopSharedDirectoryRead audit event.
func (s *WindowsService) onSharedDirectoryReadResponse(
	ctx context.Context,
	emitter events.Emitter,
	id *tlsca.Identity,
	sid string,
	desktopAddr string,
	m tdp.SharedDirectoryReadResponse,
) {
	var did directoryID
	var path string
	var offset uint64
	var name directoryName
	code := libevents.DesktopSharedDirectoryReadCode
	// Gather info from the audit cache
	info, ok := s.auditCache.GetReadRequestInfo(sessionID(sid), completionID(m.CompletionID))
	if ok {
		did = info.directoryID
		// Only search for the directory name if we retrieved the directoryID from the audit cache.
		name, ok = s.auditCache.GetName(sessionID(sid), did)
		if !ok {
			code = libevents.DesktopSharedDirectoryReadCorruptedCode
			name = events.UnknownEvent
			s.cfg.Log.Warnf("failed to find a directory name corresponding to sessionID(%v), directoryID(%v)", sid, did)
		}
		path = info.path
		offset = info.offset
	} else {
		code = libevents.DesktopSharedDirectoryReadCorruptedCode
		path = events.UnknownEvent
		name = events.UnknownEvent
		s.cfg.Log.Warnf("failed to find audit information corresponding to sessionID(%v), completionID(%v)", sid, m.CompletionID)
	}

	event := &events.DesktopSharedDirectoryRead{
		Metadata: events.Metadata{
			Type:        libevents.DesktopSharedDirectoryReadEvent,
			Code:        code,
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
		Status:        statusFromErrCode(m.ErrCode),
		DesktopAddr:   desktopAddr,
		DirectoryName: string(name),
		DirectoryID:   uint32(did),
		Path:          path,
		Length:        m.ReadDataLength,
		Offset:        offset,
	}

	s.emit(ctx, emitter, event)
}

// onSharedDirectoryWriteRequest adds WriteRequestInfo to the auditCache.
func (s *WindowsService) onSharedDirectoryWriteRequest(sid string, m tdp.SharedDirectoryWriteRequest) {
	s.auditCache.SetWriteRequestInfo(sessionID(sid), completionID(m.CompletionID), writeRequestInfo{
		directoryID: directoryID(m.DirectoryID),
		path:        m.Path,
		offset:      m.Offset,
	})
}

// onSharedDirectoryWriteResponse emits a DesktopSharedDirectoryWrite audit event.
func (s *WindowsService) onSharedDirectoryWriteResponse(
	ctx context.Context,
	emitter events.Emitter,
	id *tlsca.Identity,
	sid string,
	desktopAddr string,
	m tdp.SharedDirectoryWriteResponse,
) {
	var did directoryID
	var path string
	var offset uint64
	var name directoryName
	code := libevents.DesktopSharedDirectoryWriteCode
	// Gather info from the audit cache
	info, ok := s.auditCache.GetWriteRequestInfo(sessionID(sid), completionID(m.CompletionID))
	if ok {
		did = info.directoryID
		// Only search for the directory name if we retrieved the directoryID from the audit cache.
		name, ok = s.auditCache.GetName(sessionID(sid), did)
		if !ok {
			code = libevents.DesktopSharedDirectoryWriteCorruptedCode
			name = events.UnknownEvent
			s.cfg.Log.Warnf("failed to find a directory name corresponding to sessionID(%v), directoryID(%v)", sid, did)
		}
		path = info.path
		offset = info.offset
	} else {
		code = libevents.DesktopSharedDirectoryWriteCorruptedCode
		path = events.UnknownEvent
		name = events.UnknownEvent
		s.cfg.Log.Warnf("failed to find audit information corresponding to sessionID(%v), completionID(%v)", sid, m.CompletionID)
	}

	event := &events.DesktopSharedDirectoryWrite{
		Metadata: events.Metadata{
			Type:        libevents.DesktopSharedDirectoryWriteEvent,
			Code:        code,
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
		Status:        statusFromErrCode(m.ErrCode),
		DesktopAddr:   desktopAddr,
		DirectoryName: string(name),
		DirectoryID:   uint32(did),
		Path:          path,
		Length:        m.BytesWritten,
		Offset:        offset,
	}

	s.emit(ctx, emitter, event)
}

func (s *WindowsService) emit(ctx context.Context, emitter events.Emitter, event events.AuditEvent) {
	if err := emitter.EmitAuditEvent(ctx, event); err != nil {
		s.cfg.Log.WithError(err).Errorf("Failed to emit audit event %v", event)
	}
}

func statusFromErrCode(errCode uint32) events.Status {
	success := errCode == tdp.ErrCodeNil

	// early return for most common case
	if success {
		return events.Status{
			Success:     success,
			UserMessage: succeededSatusMessage,
		}
	}

	msg := unknownErrStatusMsg
	switch errCode {
	case tdp.ErrCodeFailed:
		msg = failedStatusMessage
	case tdp.ErrCodeDoesNotExist:
		msg = doesNotExistStatusMessage
	case tdp.ErrCodeAlreadyExists:
		msg = alreadyExistsStatusMessage
	}

	return events.Status{
		Success:     success,
		Error:       msg,
		UserMessage: msg,
	}

}

const (
	succeededSatusMessage      = "success"
	failedStatusMessage        = "operation failed"
	doesNotExistStatusMessage  = "item does not exist"
	alreadyExistsStatusMessage = "item already exists"
	unknownErrStatusMsg        = "unknown error"
)
