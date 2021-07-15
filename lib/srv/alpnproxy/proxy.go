/*
Copyright 2020-2021 Gravitational, Inc.

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

package alpnproxy

import (
	"context"
	"crypto/tls"
	"net"
	"strings"
	"sync"

	"github.com/gravitational/teleport/lib/tlsca"
	"github.com/gravitational/teleport/lib/utils"

	"github.com/gravitational/trace"
	"github.com/sirupsen/logrus"
)

const (
	KubeSNIPrefix = "kube."
)

// ProxyConfig  is the configuration for an ALPN proxy server.
type ProxyConfig struct {
	// Listener is a listener to serve requests on.
	Listener net.Listener
	// TLSConfig specifies the TLS configuration used by the Proxy server.
	TLSConfig *tls.Config
	// Router contains definition of protocol routing and handlers description.
	Router *Router
	// Log is used for logging.
	Log logrus.FieldLogger
}

// NewRouter creates a ALPN new router.
func NewRouter() *Router {
	return &Router{
		alpnHandlers: make(map[string]*HandlerDecs),
	}
}

// Router contains information about protocol handlers and routing rules.
type Router struct {
	alpnHandlers       map[string]*HandlerDecs
	kubeHandler        *HandlerDecs
	databaseTLSHandler *HandlerDecs
	mtx                sync.Mutex
}

// AddKubeHandler adds the handle for Kubernetes protocol (distinguishable by  "kube." SNI prefix).
func (r *Router) AddKubeHandler(handler HandlerFunc) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.kubeHandler = &HandlerDecs{
		Handler:    handler,
		ForwardTLS: true,
	}
}

// AddDBTLSHandler adds the handler for DB TLS traffic.
func (r *Router) AddDBTLSHandler(handler HandlerFunc) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.databaseTLSHandler = &HandlerDecs{
		Handler: handler,
	}
}

// Add sets the handler for DB TLS traffic.
func (r *Router) Add(desc HandlerDecs) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	for _, protocol := range desc.Protocols {
		r.alpnHandlers[protocol] = &desc
	}
}

// HandlerDecs describes the handler for particular protocols.
type HandlerDecs struct {
	// Protocols is a list of supported protocols by handler.
	Protocols []string
	// Handler is protocol handling logic.
	Handler HandlerFunc
	// ForwardTLS tells is ALPN proxy service should terminate TLS traffic or delegate the
	// TLS termination to the protocol handler (Used in Kube handler case)
	ForwardTLS bool
}

// HandlerFunc is a common function signature used to handle downstream with
// with particular ALPN protocol.
type HandlerFunc func(ctx context.Context, conn net.Conn) error

// Proxy server allows to route downstream connections based on
// TLS SNI ALPN values to particular service.
type Proxy struct {
	cfg                ProxyConfig
	supportedProtocols []string
	log                logrus.FieldLogger
	cancel             context.CancelFunc
}

// CheckAndSetDefaults checks and sets default values of ProxyConfig
func (c *ProxyConfig) CheckAndSetDefaults() error {
	if c.TLSConfig == nil {
		return trace.BadParameter("tls config missing")
	}
	if len(c.TLSConfig.Certificates) == 0 {
		return trace.BadParameter("missing certs")
	}

	if c.Listener == nil {
		return trace.BadParameter("listener missing")
	}
	if c.Log == nil {
		c.Log = logrus.WithField(trace.Component, "alpn:proxy")
	}
	return nil
}

// New creates a new instance of the Proxy.
func New(cfg ProxyConfig) (*Proxy, error) {
	if err := cfg.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	var supportedProtocols []string
	for k := range cfg.Router.alpnHandlers {
		supportedProtocols = append(supportedProtocols, k)
	}
	return &Proxy{
		cfg:                cfg,
		log:                cfg.Log,
		supportedProtocols: supportedProtocols,
	}, nil
}

// Serve starts accepting connections.
func (p *Proxy) Serve(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	p.cancel = cancel
	p.cfg.TLSConfig.NextProtos = p.supportedProtocols
	for {
		clientConn, err := p.cfg.Listener.Accept()
		if err != nil {
			if utils.IsOKNetworkError(err) || trace.IsConnectionProblem(err) {
				return nil
			}
			return trace.Wrap(err)
		}
		go func() {
			if err := p.handleConn(ctx, clientConn); err != nil {
				if err := clientConn.Close(); err != nil {
					p.log.Warnf("failed to close client connection: %v", err)
				}
				p.log.Warnf("failed to handle client connection: %v", err)
			}
		}()
	}
}

func (p *Proxy) handleConn(ctx context.Context, clientConn net.Conn) error {
	hello, conn, err := readHelloMessageWithoutTLSTermination(clientConn)
	if err != nil {
		return trace.Wrap(err)
	}

	handlerDesc, err := p.getHandlerDescBaseOnClientHelloMsg(hello)
	if err != nil {
		return trace.Wrap(err)
	}

	if !handlerDesc.ForwardTLS {
		tlsConn := tls.Server(conn, p.cfg.TLSConfig)
		if err := tlsConn.Handshake(); err != nil {
			return trace.Wrap(err)
		}
		conn = tlsConn
		if len(hello.SupportedProtos) > 0 && isDBTLSProtocol(hello.SupportedProtos[0]) {
			return p.databaseHandlerWithTLSTermination(ctx, tlsConn)
		}
		if p.isDatabaseConnection(tlsConn.ConnectionState()) {
			if p.cfg.Router.databaseTLSHandler == nil {
				return trace.BadParameter("database handle not enabled")
			}
			return p.cfg.Router.databaseTLSHandler.Handler(ctx, conn)
		}
	}

	if err = handlerDesc.Handler(ctx, conn); err != nil {
		return trace.Wrap(err)
	}
	return nil
}

func (p *Proxy) databaseHandlerWithTLSTermination(ctx context.Context, conn net.Conn) error {
	// DB Protocols like Mongo have native support for TLS thus TLS connection needs to be terminated twice.
	// First time for custom local proxy connection and the second time from Mongo protocol where TLS connection is used.

	tlsConn := tls.Server(conn, p.cfg.TLSConfig)
	if err := tlsConn.Handshake(); err != nil {
		return trace.Wrap(err)
	}
	if !p.isDatabaseConnection(tlsConn.ConnectionState()) {
		return trace.BadParameter("not database connection")
	}
	if p.cfg.Router.databaseTLSHandler == nil {
		return trace.BadParameter("database handle not enabled")
	}
	return p.cfg.Router.databaseTLSHandler.Handler(ctx, tlsConn)
}

func isDBTLSProtocol(protocol string) bool {
	switch protocol {
	case ProtocolMongoDB:
		return true
	default:
		return false
	}
}

func (p *Proxy) getHandlerDescBaseOnClientHelloMsg(clientHelloInfo *tls.ClientHelloInfo) (*HandlerDecs, error) {
	if shouldRouteToKubeService(clientHelloInfo.ServerName) {
		if p.cfg.Router.kubeHandler == nil {
			return nil, trace.BadParameter("received kube request but k8 service is disabled")
		}
		return p.cfg.Router.kubeHandler, nil
	}
	return p.getHandleDescBasedOnALPNVal(clientHelloInfo)
}

// getHandleDescBasedOnALPNVal returns the HandlerDesc base on ALPN field read from ClientHelloInfo message.
func (p *Proxy) getHandleDescBasedOnALPNVal(clientHelloInfo *tls.ClientHelloInfo) (*HandlerDecs, error) {
	protocol := ProtocolDefault
	if len(clientHelloInfo.SupportedProtos) != 0 {
		protocol = clientHelloInfo.SupportedProtos[0]
	}
	handlerDesc, ok := p.cfg.Router.alpnHandlers[protocol]
	if !ok {
		return nil, trace.BadParameter("unsupported ALPN protocol %q", protocol)
	}
	return handlerDesc, nil
}

func shouldRouteToKubeService(sni string) bool {
	return strings.HasPrefix(sni, KubeSNIPrefix)
}

// Close the Proxy server.
func (p *Proxy) Close() error {
	if p.cancel != nil {
		p.cancel()
	}
	if err := p.cfg.Listener.Close(); err != nil {
		return trace.Wrap(err)
	}
	return nil
}

// isDatabaseConnection inspects the TLS connection state and returns true
// if it's a database access connection as determined by the decoded
// identity from the client certificate.
func (p *Proxy) isDatabaseConnection(state tls.ConnectionState) bool {
	// VerifiedChains must be populated after the handshake.
	if len(state.VerifiedChains) < 1 || len(state.VerifiedChains[0]) < 1 {
		return false
	}
	identity, err := tlsca.FromSubject(state.VerifiedChains[0][0].Subject,
		state.VerifiedChains[0][0].NotAfter)
	if err != nil {
		p.log.WithError(err).Debug("Failed to decode identity from client certificate.")
		return false
	}
	return identity.RouteToDatabase.ServiceName != ""
}
