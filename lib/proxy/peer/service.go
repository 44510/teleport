// Copyright 2022 Gravitational, Inc
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package peer

import (
	"net"
	"strings"

	"github.com/gravitational/trace"
	"github.com/sirupsen/logrus"

	"github.com/gravitational/teleport/api/client/proto"
	"github.com/gravitational/teleport/api/types"
	streamutils "github.com/gravitational/teleport/api/utils/grpc/stream"
	"github.com/gravitational/teleport/lib/utils"
)

// proxyService implements the grpc ProxyService.
type proxyService struct {
	clusterDialer ClusterDialer
	log           logrus.FieldLogger
}

// DialNode opens a bidirectional stream to the requested node.
func (s *proxyService) DialNode(stream proto.ProxyService_DialNodeServer) error {
	frame, err := stream.Recv()
	if err != nil {
		return trace.Wrap(err)
	}

	if r := frame.GetDialRequest(); r != nil {
		return trace.Wrap(s.handleDialRequest(stream, r))
	} else if r := frame.GetDialAuth(); r != nil {
		return trace.Wrap(s.handleDialAuth(stream, r))
	}

	return trace.BadParameter("unknown dial request")
}

func (s *proxyService) handleDialRequest(stream proto.ProxyService_DialNodeServer, dial *proto.DialRequest) error {
	if dial.Source == nil || dial.Destination == nil {
		return trace.BadParameter("invalid dial request: source and destination must not be nil")
	}

	// Dial request must be to a node or auth.
	if dial.NodeID == "" {
		return trace.BadParameter("invalid dial request: missing node id")
	}

	_, clusterName, err := splitServerID(dial.NodeID)
	if err != nil {
		return trace.Wrap(err)
	}

	log := s.log.WithFields(logrus.Fields{
		"node":    dial.NodeID,
		"cluster": clusterName,
		"src":     dial.Source.Addr,
		"dst":     dial.Destination.Addr,
	})
	log.Debugf("Node dial request from peer.")

	source := &utils.NetAddr{
		Addr:        dial.Source.Addr,
		AddrNetwork: dial.Source.Network,
	}
	destination := &utils.NetAddr{
		Addr:        dial.Destination.Addr,
		AddrNetwork: dial.Destination.Network,
	}

	conn, err := s.clusterDialer.Dial(clusterName, DialParams{
		From:     source,
		To:       destination,
		ServerID: dial.NodeID,
		ConnType: dial.TunnelType,
	})
	if err != nil {
		return trace.Wrap(err)
	}
	return trace.Wrap(s.handleConnectionStream(stream, conn, source, destination, log))
}

func (s *proxyService) handleDialAuth(stream proto.ProxyService_DialNodeServer, dial *proto.DialAuth) error {
	if dial.ClusterName == "" {
		return trace.BadParameter("invalid dial request: cluster name must not be empty")
	}
	if dial.Source == nil || dial.Destination == nil {
		return trace.BadParameter("invalid dial request: source and destination must not be nil")
	}

	log := s.log.WithFields(logrus.Fields{
		"cluster": dial.ClusterName,
		"src":     dial.Source.Addr,
		"dst":     dial.Destination.Addr,
	})
	log.Debugf("Auth dial request from peer.")

	source := &utils.NetAddr{
		Addr:        dial.Source.Addr,
		AddrNetwork: dial.Source.Network,
	}
	destination := &utils.NetAddr{
		Addr:        dial.Destination.Addr,
		AddrNetwork: dial.Destination.Network,
	}

	conn, err := s.clusterDialer.DialAuth(dial.ClusterName, DialParams{
		From: source,
		To:   destination,
	})
	if err != nil {
		return trace.Wrap(err)
	}
	return trace.Wrap(s.handleConnectionStream(stream, conn, source, destination, log))
}

func (s *proxyService) handleConnectionStream(stream proto.ProxyService_DialNodeServer, conn net.Conn, source, destination net.Addr, log logrus.FieldLogger) error {
	if err := stream.Send(&proto.Frame{
		Message: &proto.Frame_ConnectionEstablished{
			ConnectionEstablished: &proto.ConnectionEstablished{},
		},
	}); err != nil {
		return trace.Wrap(err)
	}

	streamRW, err := streamutils.NewReadWriter(frameStream{stream: stream})
	if err != nil {
		return trace.Wrap(err)
	}

	streamConn := utils.NewTrackingConn(streamutils.NewConn(streamRW, source, destination))
	err = utils.ProxyConn(stream.Context(), streamConn, conn)
	sent, received := streamConn.Stat()
	log.Debugf("Closing dial request from peer. sent: %d received %d", sent, received)
	return trace.Wrap(err)
}

// splitServerID splits a server id in to a node id and cluster name.
func splitServerID(address string) (string, string, error) {
	split := strings.Split(address, ".")
	if len(split) == 0 || split[0] == "" {
		return "", "", trace.BadParameter("invalid server id: \"%s\"", address)
	}

	return split[0], strings.Join(split[1:], "."), nil
}

// ClusterDialer dials a node in the given cluster.
type ClusterDialer interface {
	Dial(clusterName string, request DialParams) (net.Conn, error)
	DialAuth(clusterName string, request DialParams) (net.Conn, error)
}

type DialParams struct {
	From     *utils.NetAddr
	To       *utils.NetAddr
	ServerID string
	ConnType types.TunnelType
}
